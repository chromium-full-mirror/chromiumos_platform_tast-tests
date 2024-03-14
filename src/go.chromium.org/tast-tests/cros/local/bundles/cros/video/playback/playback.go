// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package playback provides common code for video.Playback* tests.
package playback

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mafredri/cdp/protocol/media"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/devtools"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/tracing"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DecoderType represents the different video decoder types.
type DecoderType int

const (
	// Hardware means hardware-accelerated video decoding.
	Hardware DecoderType = iota
	// Software - Any software-based video decoder (e.g. ffmpeg, libvpx).
	Software
)

const (
	// Time to sleep while collecting data.
	// The time to wait just after stating to play video so that CPU usage gets stable.
	stabilizationDuration = 5 * time.Second
	// TraceConfigFile is the perfetto config file to profile the scheduler events.
	TraceConfigFile = "perfetto_tbm_traced_probes.pbtxt"
	// GPUThreadSchedSQLFile is the sql script to count the number of context
	// switches and its waiting duration from the perfetto output.
	GPUThreadSchedSQLFile = "gpu_thread_sched.sql"

	// Video Element in the page to play a video.
	videoElement = "document.getElementsByTagName('video')[0]"

	// SuspendSystemTimeout is the timeout to do a single suspend, chrome reconnection, and additional checks.
	// SuspendSystem usually finishes within 10 seconds. Give it 30 seconds max to finish suspend/resume cycle.
	SuspendSystemTimeout = 30 * time.Second
	// SuspendSystemInterval is the interval between each suspendSystem call.
	SuspendSystemInterval = 15 * time.Second
)

type contextSwitchStat struct {
	count       uint64
	avgDuration time.Duration
}

// PerfSetting is the setting for playback perf configuration.
type PerfSetting struct {
	// If set, trace system evens using perfetto during playback.
	PerfTracing bool
	// If set, we record steady-state metrics: metrics that require convergence to within a specific tolerance in order to reduce noise across test runs.
	MeasureSteadyStateMetrics bool
	// If set, uses a longer video sequence which allows for measuring Media Devtools "playback roughness".
	MeasureRoughness bool
}

// Config is the configuration to run Playback test.
type Config struct {
	FileName    string
	DecoderType DecoderType
	BrowserType browser.Type
	// Creates a layout of |Grid.Width| x |Grid.Height| videos for playback. Values less than 1 are clamped to a grid of 1x1.
	Grid coords.Size
	// If set, run performance measurement while playing the video.
	PerfMeasurement bool
	// Setting to configure additional performance measurements.
	PerfSetting PerfSetting
	// If set, let whole system go to suspend state while playing the video.
	SuspendResume bool
	// The video playback duration, if set to 0, it runs indefinitely or until global timeout is hit.
	Duration time.Duration
}

// RunTest measures a number of performance metrics while playing a video with or without hardware acceleration as per DecoderType.
func RunTest(ctx context.Context, s *testing.State, tconn *chrome.TestConn, config Config) {
	// Save 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	s.Log("Starting playback")
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	url := server.URL + "/video.html"
	conn, br, browserCleanup, err := browserfixt.SetUpWithURL(ctx, cr, config.BrowserType, url)
	if err != nil {
		s.Fatal("Failed to setup browser: ", err)
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API: ", err)
	}
	defer browserCleanup(cleanupCtx)
	// Close the tab and release the resource.
	defer func(ctx context.Context) {
		// Connection maybe tampered (e.g. suspend/resume) and we need to re-establish connection.
		conn, err := reconnectToBrowser(ctx, cr, config.BrowserType)
		if err != nil {
			s.Fatal("Failed to reconnect to browser")
		}
		conn.CloseTarget(ctx)
		conn.Close()
	}(cleanupCtx)

	observer, err := conn.GetMediaPropertiesChangedObserver(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve DevTools Media messages: ", err)
	}

	// The page is already rendered with 1 video element by default.
	defaultGridSize := 1
	gridSize := config.Grid.Width * config.Grid.Height
	if gridSize > defaultGridSize {
		if err := conn.Call(ctx, nil, "setGridSize", config.Grid.Width, config.Grid.Height); err != nil {
			s.Fatal("Failed to adjust the grid size: ", err)
		}
	}

	// Wait until video element(s) are loaded.
	exprn := fmt.Sprintf("document.getElementsByTagName('video').length == %d", int(math.Max(1.0, float64(gridSize))))
	if err := conn.WaitForExpr(ctx, exprn); err != nil {
		s.Fatal("Failed to wait for video element loading: ", err)
	}

	// Rotate the display to landscape-primary.
	if _, err := display.GetInternalInfo(ctx, tconn); err == nil {
		if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
			s.Fatal("Failed to set display to landscape-primary orientation: ", err)
		}
	}

	// Maximize the playback window.
	w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "ChromeOS Video Test")
	if err != nil {
		s.Fatal("Failed to find the window that contains the video: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the window that contains the video: ", err)
	}

	if config.PerfMeasurement {
		// Wait for CPU to cool down before playing the video and recording metrics.
		if err := cpu.Cooldown(ctx); err != nil {
			s.Fatal("Failed to wait for cpu to cooldown: ", err)
		}
	}

	// Start playing the video.
	if err := conn.Call(ctx, nil, "playRepeatedly", config.FileName); err != nil {
		s.Fatal("Failed to start video: ", err)
	}

	// Wait until videoElement has advanced so that chrome:media-internals has time to fill in their fields.
	if err := conn.WaitForExpr(ctx, videoElement+".currentTime > 1"); err != nil {
		s.Fatal("Failed waiting for video to advance playback: ", err)
	}
	isPlatform, decoderName, err := devtools.GetVideoDecoder(ctx, observer, url)
	if err != nil {
		s.Fatal("Failed to parse Media DevTools: ", err)
	}
	if config.DecoderType == Hardware && !isPlatform {
		s.Fatal("Hardware decoding accelerator was expected but wasn't used")
	}
	if config.DecoderType == Software && isPlatform {
		s.Fatal("Software decoding was expected but wasn't used")
	}
	s.Log("decoderName: ", decoderName)

	if config.PerfMeasurement {
		s.Logf("Playing %v video while measuring performance", config.Duration)
		if err := measurePerformance(ctx, measureParams{
			conn:                  conn,
			tconn:                 tconn,
			bTconn:                bTconn,
			url:                   url,
			observer:              observer,
			config:                config.PerfSetting,
			duration:              config.Duration,
			traceConfigPath:       s.DataPath(TraceConfigFile),
			gpuThreadSchedSQLPath: s.DataPath(GPUThreadSchedSQLFile),
		}); err != nil {
			s.Fatal("Video playback failed: ", err)
		}
	}
	// Test video playback continuously after system goes through suspend/resume cycle.
	if config.SuspendResume {
		s.Logf("Playing %v video while performancing system suspend/resume", config.Duration)
		if err := suspendResume(ctx, cr, config, s.TestName()); err != nil {
			s.Fatal("Video playback failed: ", err)
		}
	}
	if !config.PerfMeasurement && !config.SuspendResume {
		s.Logf("Sleeping for %v duration while playing video", config.Duration)
		select {
		case <-ctx.Done():
			s.Fatal("Context timeout: ", ctx.Err())
		case <-time.After(config.Duration):
			return
		}
	}
}

// getPlayingTime returns the currentTime from the video element.
func getPlayingTime(ctx context.Context, conn *chrome.Conn) (float64, error) {
	var curTime float64
	if err := conn.Eval(ctx, videoElement+".currentTime", &curTime); err != nil {
		return 0, errors.Wrap(err, "failed to get current playing time")
	}
	return curTime, nil
}

func reconnectToBrowser(ctx context.Context, cr *chrome.Chrome, browserType browser.Type) (*chrome.Conn, error) {
	// Reconnect to Chrome.
	testing.ContextLog(ctx, "Reconnect to browser connection")
	if err := cr.Reconnect(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to Chrome")
	}
	// Reconnect to browser.
	br, _, err := browserfixt.Connect(ctx, cr, browserType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to browser")
	}
	conn, err := br.NewConnForTarget(ctx, func(t *chrome.Target) bool {
		return strings.HasSuffix(t.URL, "video.html")
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish browser connection")
	}
	testing.ContextLog(ctx, "Browser connection is established")
	return conn, nil
}

// suspendSystem suspends the system and checks the validity of syslog and video after system resumes.
func suspendSystem(ctx context.Context, cr *chrome.Chrome, reader *syslog.Reader, config Config, testName string) error {
	// Check syslog for GPU hangs and decoding errors before we start suspend/resume.
	if err := graphics.CheckSysLog(ctx, testName, reader); err != nil {
		return errors.Wrap(err, "syslog signature found")
	}
	out, err := testexec.CommandContext(ctx, "suspend_stress_test", "--count", "1").Output(testexec.DumpLogOnError)
	testing.ContextLogf(ctx, "%v", string(out))
	if err != nil {
		return errors.Wrap(err, "suspend_stress_test failed")
	}
	if match := regexp.MustCompile(`(?m)^(Suspend failed.*)$`).FindSubmatch(out); len(match) > 0 {
		return errors.Errorf("suspend_stress_test failed: %v", string(match[1]))
	}
	conn, err := reconnectToBrowser(ctx, cr, config.BrowserType)
	// Check |currentTime| variable is changing.
	originalPlayingTime, err := getPlayingTime(ctx, conn)
	if err != nil {
		return errors.Wrap(err, "failed to get original playing time")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		playingTime, err := getPlayingTime(ctx, conn)
		if err != nil {
			return errors.Wrap(err, "failed to get playing time")
		}
		if playingTime == originalPlayingTime {
			return errors.Errorf("playing time %v is the same as original time %v", playingTime, originalPlayingTime)
		}
		return nil
	}, nil); err != nil {
		return errors.Wrap(err, "video playing time is not advancing")
	}
	// Check syslog for GPU hangs and decoding errors after the video starts playing.
	if err := graphics.CheckSysLog(ctx, testName, reader); err != nil {
		return errors.Wrap(err, "syslog signature found")
	}
	return nil
}

// suspendResume tests video playback continuity after system goes through suspend/resume cycle.
func suspendResume(ctx context.Context, cr *chrome.Chrome, config Config, testName string) (resultErr error) {
	var wg sync.WaitGroup
	wg.Add(1)
	errChan := make(chan error)
	quit := make(chan bool)

	// Start reading the syslog so we can stop the tests as soon as any GPU hangs/decode errors are found.
	reader, err := syslog.NewReader(ctx, syslog.Severities(syslog.Info, syslog.Warning, syslog.Err))
	if err != nil {
		return errors.Wrap(err, "failed to get syslog reader")
	}
	defer reader.Close()
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-quit:
				return
			default:
				testing.ContextLogf(ctx, "Executing [%v] suspend ", i)
			}

			suspendCtx, cancel := context.WithTimeout(ctx, SuspendSystemTimeout)
			defer cancel()
			if err := suspendSystem(suspendCtx, cr, reader, config, testName); err != nil {
				errChan <- errors.Wrapf(err, "suspend [%v] failed", i)
				return
			}
			if suspendCtx.Err() != nil {
				errChan <- errors.Wrapf(err, "suspend/resume cycle took more than %v", SuspendSystemTimeout)
				return
			}
			// GoBigSleepLint: Add a suspend intervals so video can progress.
			if err := testing.Sleep(ctx, SuspendSystemInterval); err != nil {
				errChan <- errors.Wrap(err, "failed to sleep after suspend")
				return
			}
		}
	}()

	// Wait either suspend fails or playbackDuration is passed.
	select {
	case resultErr = <-errChan:
		break
	case <-ctx.Done():
		quit <- true
		return ctx.Err()
	case <-time.After(config.Duration):
		testing.ContextLog(ctx, "playback duration hit, stopping suspend_resume loop")
		quit <- true
	}
	wg.Wait()
	return
}

type measureParams struct {
	conn     *chrome.Conn
	tconn    *chrome.TestConn
	bTconn   *chrome.TestConn
	url      string // URL to the video
	observer media.PlayerPropertiesChangedClient
	config   PerfSetting
	duration time.Duration // Duration of the measurement.

	traceConfigPath       string
	gpuThreadSchedSQLPath string
}

// measurePerformance collects video playback performance playing a video with either SW or HW decoder.
func measurePerformance(ctx context.Context, params measureParams) error {
	p := graphics.NewThreadSafePerfValues()
	// Save the perf result to OutDir even if something wrong while measuring the performance.
	defer func() {
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			return
		}
		p.GetUnderlyingValues().Save(outDir)
	}()

	const decodeHistogram = "Media.MojoVideoDecoder.Decode"
	initDecodeHistogram, err := metrics.GetHistogram(ctx, params.bTconn, decodeHistogram)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}
	const platformdecodeHistogram = "Media.PlatformVideoDecoding.Decode"
	initPlatformdecodeHistogram, err := metrics.GetHistogram(ctx, params.tconn, platformdecodeHistogram)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}
	const overlaysHistogram = "Viz.DisplayCompositor.OverlayStrategy"
	initOverlaysHistogram, err := metrics.GetHistogram(ctx, params.tconn, overlaysHistogram)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}
	// minPromotedOverlayValue and maxPromotedOverlayValue are 2 and 5 since the buckets representing
	// samples promoted to overlays in this histogram are
	// Fullscreen: 2
	// SingleOnTop: 3
	// Underlay: 4
	// Underlay Cast: 5
	minPromotedOverlayValue := 2
	maxPromotedOverlayValue := 5

	measurementDuration := params.duration

	var roughness float64
	var gpuErr, i915IRQErr, cStateErr, cpuErr, fdErr, wakeupErr, dramErr, batErr, roughnessErr error
	var wg sync.WaitGroup
	wg.Add(8)
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		i915IRQErr = graphics.MeasureI915IRQs(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cStateErr = graphics.MeasurePackageCStateCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cpuErr = graphics.MeasureCPUUsageAndPower(ctx, 0 /*stabilizationDuration*/, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		fdErr = graphics.MeasureFdCount(ctx, graphics.GPUProcess, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		wakeupErr = graphics.MeasureThreadPoolUnnecessaryWakeups(ctx, params.bTconn, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		dramErr = graphics.MeasureDRAMBandwidth(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		batErr = graphics.MeasureSystemPowerConsumption(ctx, params.tconn, measurementDuration, p)
	}()
	if params.config.MeasureSteadyStateMetrics {
		wg.Add(1)

		go func() {
			defer wg.Done()
			batErr = graphics.MeasureSteadyStateSystemPowerConsumption(
				ctx,
				params.tconn,
				100,                  /*numSamples*/
				500*time.Millisecond, /*samplePeriod*/
				0.15,                 /*tolerance*/
				measurementDuration,  /*minDuration*/
				p,
			)
		}()
	}
	if params.config.MeasureRoughness {
		wg.Add(1)

		go func() {
			defer wg.Done()
			// If the video sequence is not long enough, roughness won't be provided by
			// Media Devtools and this call will timeout.
			roughness, roughnessErr = devtools.GetVideoPlaybackRoughness(ctx, params.observer, params.url)
		}()
	}

	wg.Wait()
	if gpuErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
	}
	if i915IRQErr != nil {
		return errors.Wrap(i915IRQErr, "failed to measure i915 IRQs/s")
	}
	if cStateErr != nil {
		return errors.Wrap(cStateErr, "failed to measure Package C-State residency")
	}
	if cpuErr != nil {
		return errors.Wrap(cpuErr, "failed to measure CPU/Package power")
	}
	if fdErr != nil {
		return errors.Wrap(fdErr, "failed to measure open FD count")
	}
	if wakeupErr != nil {
		return errors.Wrap(wakeupErr, "failed to measure unnecessary wakeups of ThreadPool")
	}
	if dramErr != nil {
		return errors.Wrap(dramErr, "failed to measure DRAM bandwidth consumption")
	}
	if batErr != nil {
		return errors.Wrap(batErr, "failed to measure system power consumption")
	}
	if roughnessErr != nil {
		return errors.Wrap(roughnessErr, "failed to measure playback roughness")
	}

	if err := graphics.UpdatePerfMetricFromHistogram(ctx, params.bTconn, decodeHistogram, initDecodeHistogram, p.GetUnderlyingValues(), "video_decode_delay"); err != nil {
		return errors.Wrap(err, "failed to calculate Decode perf metric")
	}
	if err := graphics.UpdatePerfMetricFromHistogram(ctx, params.tconn, platformdecodeHistogram, initPlatformdecodeHistogram, p.GetUnderlyingValues(), "platform_video_decode_delay"); err != nil {
		return errors.Wrap(err, "failed to calculate Platform Decode perf metric")
	}
	if err := graphics.UpdateOverlaysMetricFromHistogram(ctx, params.tconn, overlaysHistogram, initOverlaysHistogram, minPromotedOverlayValue, maxPromotedOverlayValue, p.GetUnderlyingValues(), "overlays"); err != nil {
		return errors.Wrap(err, "failed to calculate overlays metric")
	}

	if err := sampleDroppedFrames(ctx, params.conn, p.GetUnderlyingValues()); err != nil {
		return errors.Wrap(err, "failed to get dropped frames and percentage")
	}

	if params.config.MeasureRoughness {
		p.Set(perf.Metric{
			Name:      "roughness",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, float64(roughness))
	}

	if params.config.PerfTracing {
		gpuCSStat, gpuMainCSStat, traceErr := measureContextSwitch(ctx, measurementDuration, params.traceConfigPath, params.gpuThreadSchedSQLPath)
		if traceErr != nil {
			return errors.Wrap(traceErr, "failed to measure CPU sched events")
		}

		p.Set(perf.Metric{
			Name:      "context_switches_in_gpu_process_cnt",
			Unit:      "count",
			Direction: perf.SmallerIsBetter,
		}, float64(gpuCSStat.count))
		p.Set(perf.Metric{
			Name:      "context_switches_in_gpu_process_avg_duration",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, float64(gpuCSStat.avgDuration.Milliseconds()))
		p.Set(perf.Metric{
			Name:      "context_switches_in_gpu_process_for_gpu_main_thread_cnt",
			Unit:      "count",
			Direction: perf.SmallerIsBetter,
		}, float64(gpuMainCSStat.count))
		p.Set(perf.Metric{
			Name:      "context_switches_in_gpu_process_for_gpu_main_thread_avg_duration",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, float64(gpuMainCSStat.avgDuration.Milliseconds()))
	}
	if err := params.conn.Eval(ctx, videoElement+".pause()", nil); err != nil {
		return errors.Wrap(err, "failed to stop video")
	}
	return nil
}

// sampleDroppedFrames obtains the number of decoded and dropped frames.
func sampleDroppedFrames(ctx context.Context, conn *chrome.Conn, p *perf.Values) error {
	var decodedFrameCount, droppedFrameCount int64
	if err := conn.Eval(ctx, videoElement+".getVideoPlaybackQuality().totalVideoFrames", &decodedFrameCount); err != nil {
		return errors.Wrap(err, "failed to get number of decoded frames")
	}
	if err := conn.Eval(ctx, videoElement+".getVideoPlaybackQuality().droppedVideoFrames", &droppedFrameCount); err != nil {
		return errors.Wrap(err, "failed to get number of dropped frames")
	}

	var droppedFramePercent float64
	if decodedFrameCount != 0 {
		droppedFramePercent = 100.0 * float64(droppedFrameCount) / float64(decodedFrameCount)
	} else {
		testing.ContextLog(ctx, "No decoded frames; setting dropped percent to 100")
		droppedFramePercent = 100.0
	}

	p.Set(perf.Metric{
		Name:      "dropped_frames",
		Unit:      "frames",
		Direction: perf.SmallerIsBetter,
	}, float64(droppedFrameCount))
	p.Set(perf.Metric{
		Name:      "dropped_frames_percent",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, droppedFramePercent)

	testing.ContextLogf(ctx, "Dropped frames: %d (%f%%)", droppedFrameCount, droppedFramePercent)

	return nil
}

// measureContextSwitch measure the number of context switches in GPU process and its average waiting duration.
// gpu represents the values of all the threads in GPU process.
// gpuMain represents the values of the GPU main thread.
func measureContextSwitch(ctx context.Context, measurementDuration time.Duration, traceConfigPath, gpuThreadSchedSQLPath string) (gpu, gpuMain contextSwitchStat, err error) {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	// GoBigSleepLint: sleep to stabilize CPU usage.
	if err := testing.Sleep(ctx, stabilizationDuration); err != nil {
		return gpu, gpuMain, err
	}

	testing.ContextLog(ctx, "Tracing scheduler events")
	// Record system events for |measurementDuration|.
	sess, err := tracing.StartSession(ctx, traceConfigPath)
	if err != nil {
		return gpu, gpuMain, errors.Wrap(err, "failed to start tracing")
	}
	defer sess.Finalize(ctxForCleanup)
	// Stop tracing even if context deadline exceeds during sleep.
	stopped := false
	defer func() {
		if !stopped {
			sess.Stop()
		}
	}()

	// GoBigSleepLint: sleep to stabilize CPU usage.
	if err := testing.Sleep(ctx, measurementDuration); err != nil {
		return gpu, gpuMain, errors.Wrap(err, "failed to sleep to wait for the tracing session")
	}
	stopped = true
	if err := sess.Stop(); err != nil {
		return gpu, gpuMain, errors.Wrap(err, "failed to stop tracing")
	}
	testing.ContextLog(ctx, "Completed tracing events")

	results, err := sess.RunQuery(ctx, gpuThreadSchedSQLPath)
	if err != nil {
		return gpu, gpuMain, errors.Wrap(err, "failed in querying")
	}

	switchMap := make(map[string]int)

	var mainSwitches, mainRunnableCnt, mainSumRunnableDur uint64 = 0, 0, 0
	var switches, runnableCnt, sumRunnableDur uint64 = 0, 0, 0
	for _, res := range results[1:] { // Skip, the first line, "ts","dur","state","tid","name", "is_main_thread".
		const (
			tsIdx = iota
			durIdx
			stateIdx
			tidIdx
			nameIdx
			mainThreadIdx
		)

		isMainThread := false
		if res[mainThreadIdx] == "1" {
			isMainThread = true
		}
		switch res[stateIdx] {
		case "Running":
			switches++
			thName := res[nameIdx]
			switchMap[thName]++
			if isMainThread {
				mainSwitches++
			}
		case "R": // Runnable
			dur, err := strconv.Atoi(res[durIdx])
			if err != nil {
				return gpu, gpuMain, errors.Wrapf(err, "failed to convert to integer, %s", res[durIdx])
			}
			if dur == -1 {
				// dur is -1 if tracing terminates while a thread is in the Runnable state.
				continue
			}
			runnableCnt++
			sumRunnableDur += uint64(dur)
			if isMainThread {
				mainRunnableCnt++
				mainSumRunnableDur += uint64(dur)
			}
		}
	}

	keys := make([]string, 0, len(switchMap))
	for key := range switchMap {
		keys = append(keys, key)
	}
	// Sort the thread names according to the number of context switches, from more to
	// fewer.
	sort.SliceStable(keys, func(i, j int) bool {
		return switchMap[keys[i]] > switchMap[keys[j]]
	})

	testing.ContextLog(ctx, "Switches in GPU process: ", switches)
	const maxPrintKeys = 8
	for i, key := range keys {
		if i > maxPrintKeys {
			break
		}
		testing.ContextLogf(ctx, "%15s: %d", key, switchMap[key])
	}

	gpu.count = switches
	if runnableCnt > 0 {
		gpu.avgDuration = time.Duration(sumRunnableDur/runnableCnt) * time.Microsecond
	}
	gpuMain.count = mainSwitches
	if mainRunnableCnt > 0 {
		gpuMain.avgDuration = time.Duration(mainSumRunnableDur/mainRunnableCnt) * time.Microsecond
	}
	return gpu, gpuMain, nil
}
