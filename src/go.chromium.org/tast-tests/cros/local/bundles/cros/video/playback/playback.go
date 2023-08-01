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
	"sort"
	"strconv"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/devtools"
	"go.chromium.org/tast-tests/cros/local/media/logging"
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
	// The time to wait after CPU is stable so as to measure solid metric values.
	measurementDuration = 25 * time.Second

	// TraceConfigFile is the perfetto config file to profile the scheduler events.
	TraceConfigFile = "perfetto_tbm_traced_probes.pbtxt"
	// GPUThreadSchedSQLFile is the sql script to count the number of context
	// switches and its waiting duration from the perfetto output.
	GPUThreadSchedSQLFile = "gpu_thread_sched.sql"

	// Video Element in the page to play a video.
	videoElement = "document.getElementsByTagName('video')[0]"
)

type contextSwitchStat struct {
	count       uint64
	avgDuration time.Duration
}

// RunTest measures a number of performance metrics while playing a video with
// or without hardware acceleration as per decoderType.
func RunTest(ctx context.Context, s *testing.State, cs ash.ConnSource, tconn, bTconn *chrome.TestConn, videoName string, decoderType DecoderType, gridWidth, gridHeight int, perfTracing, measureRoughness bool) {
	vl, err := logging.NewVideoLogger()
	if err != nil {
		s.Fatal("Failed to set values for verbose logging")
	}
	defer vl.Close()

	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute device: ", err)
	}
	defer crastestclient.Unmute(ctx)

	s.Log("Starting playback")
	if err = measurePerformance(ctx, s, cs, tconn, bTconn, s.DataFileSystem(), videoName, decoderType, gridWidth, gridHeight, perfTracing, measureRoughness, s.OutDir()); err != nil {
		s.Fatal("Playback test failed: ", err)
	}
}

// measurePerformance collects video playback performance playing a video with
// either SW or HW decoder.
func measurePerformance(ctx context.Context, s *testing.State, cs ash.ConnSource, tconn, bTconn *chrome.TestConn, fileSystem http.FileSystem, videoName string,
	decoderType DecoderType, gridWidth, gridHeight int, perfTracing, measureRoughness bool, outDir string) error {
	server := httptest.NewServer(http.FileServer(fileSystem))
	defer server.Close()

	url := server.URL + "/video.html"
	conn, err := cs.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "failed to open video page")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	observer, err := conn.GetMediaPropertiesChangedObserver(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve DevTools Media messages")
	}

	// The page is already rendered with 1 video element by default.
	defaultGridSize := 1
	if gridWidth*gridHeight > defaultGridSize {
		if err := conn.Call(ctx, nil, "setGridSize", gridWidth, gridHeight); err != nil {
			return errors.Wrap(err, "failed to adjust the grid size")
		}
	}

	// Wait until video element(s) are loaded.
	exprn := fmt.Sprintf("document.getElementsByTagName('video').length == %d", int(math.Max(1.0, float64(gridWidth*gridHeight))))
	if err := conn.WaitForExpr(ctx, exprn); err != nil {
		return errors.Wrap(err, "failed to wait for video element loading")
	}

	// For consistency across test runs, let's try to put the UI in a known state:
	// rotate the display to landscape-primary and maximize the browser window.
	if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to set display to landscape-primary orientation")
	}
	w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "ChromeOS Video Test")
	if err != nil {
		s.Fatal("Failed to find the window that contains the video: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the window that contains the video: ", err)
	}

	// Wait until CPU is idle enough before playing the video.
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		return err
	}

	// TODO(b/183044442): before playing and measuring, we should probably ensure
	// that the UI is in a known state.
	if err := conn.Call(ctx, nil, "playRepeatedly", videoName); err != nil {
		return errors.Wrap(err, "failed to start video")
	}

	// Wait until videoElement has advanced so that chrome:media-internals has
	// time to fill in their fields.
	if err := conn.WaitForExpr(ctx, videoElement+".currentTime > 1"); err != nil {
		return errors.Wrap(err, "failed waiting for video to advance playback")
	}

	isPlatform, decoderName, err := devtools.GetVideoDecoder(ctx, observer, url)
	if err != nil {
		return errors.Wrap(err, "failed to parse Media DevTools")
	}
	if decoderType == Hardware && !isPlatform {
		return errors.New("hardware decoding accelerator was expected but wasn't used")
	}
	if decoderType == Software && isPlatform {
		return errors.New("software decoding was expected but wasn't used")
	}
	testing.ContextLog(ctx, "decoderName: ", decoderName)

	p := perf.NewValues()

	const decodeHistogram = "Media.MojoVideoDecoder.Decode"
	initDecodeHistogram, err := metrics.GetHistogram(ctx, bTconn, decodeHistogram)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}
	const platformdecodeHistogram = "Media.PlatformVideoDecoding.Decode"
	initPlatformdecodeHistogram, err := metrics.GetHistogram(ctx, tconn, platformdecodeHistogram)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}
	const overlaysHistogram = "Viz.DisplayCompositor.OverlayStrategy"
	initOverlaysHistogram, err := metrics.GetHistogram(ctx, tconn, overlaysHistogram)
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

	var roughness float64
	var gpuCSStat, gpuMainCSStat contextSwitchStat
	var gpuErr, cStateErr, cpuErr, fdErr, wakeupErr, dramErr, batErr, roughnessErr, traceErr error
	var wg sync.WaitGroup
	wg.Add(7)
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, measurementDuration, p)
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
		fdErr = graphics.MeasureFdCount(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		wakeupErr = graphics.MeasureThreadPoolUnnecessaryWakeups(ctx, bTconn, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		dramErr = graphics.MeasureDRAMBandwidth(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		batErr = graphics.MeasureSystemPowerConsumption(ctx, tconn, measurementDuration, p)
	}()
	if measureRoughness {
		wg.Add(1)

		go func() {
			defer wg.Done()
			// If the video sequence is not long enough, roughness won't be provided by
			// Media Devtools and this call will timeout.
			roughness, roughnessErr = devtools.GetVideoPlaybackRoughness(ctx, observer, url)
		}()
	}
	if perfTracing {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gpuCSStat, gpuMainCSStat, traceErr = measureContextSwitch(ctx, s)
		}()
	}

	wg.Wait()
	if gpuErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
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
	if traceErr != nil {
		return errors.Wrap(traceErr, "failed to measure CPU sched events")
	}

	if err := graphics.UpdatePerfMetricFromHistogram(ctx, bTconn, decodeHistogram, initDecodeHistogram, p, "video_decode_delay"); err != nil {
		return errors.Wrap(err, "failed to calculate Decode perf metric")
	}
	if err := graphics.UpdatePerfMetricFromHistogram(ctx, tconn, platformdecodeHistogram, initPlatformdecodeHistogram, p, "platform_video_decode_delay"); err != nil {
		return errors.Wrap(err, "failed to calculate Platform Decode perf metric")
	}
	if err := graphics.UpdateOverlaysMetricFromHistogram(ctx, tconn, overlaysHistogram, initOverlaysHistogram, minPromotedOverlayValue, maxPromotedOverlayValue, p, "overlays"); err != nil {
		return errors.Wrap(err, "failed to calculate overlays metric")
	}

	if err := sampleDroppedFrames(ctx, conn, p); err != nil {
		return errors.Wrap(err, "failed to get dropped frames and percentage")
	}

	if measureRoughness {
		p.Set(perf.Metric{
			Name:      "roughness",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, float64(roughness))
	}

	if perfTracing {
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
	if err := conn.Eval(ctx, videoElement+".pause()", nil); err != nil {
		return errors.Wrap(err, "failed to stop video")
	}

	p.Save(outDir)
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
func measureContextSwitch(ctx context.Context, s *testing.State) (gpu, gpuMain contextSwitchStat, err error) {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	// GoBigSleepLint: sleep to stabilize CPU usage.
	if err := testing.Sleep(ctx, stabilizationDuration); err != nil {
		return gpu, gpuMain, err
	}

	testing.ContextLog(ctx, "Tracing scheduler events")
	// Record system events for |measurementDuration|.
	sess, err := tracing.StartSession(ctx, s.DataPath(TraceConfigFile))
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

	results, err := sess.RunQuery(ctx, s.DataPath(GPUThreadSchedSQLFile))
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
