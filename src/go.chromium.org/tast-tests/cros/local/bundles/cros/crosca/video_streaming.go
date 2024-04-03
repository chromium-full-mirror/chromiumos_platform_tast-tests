// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crosca contains a collections of performance tests
package crosca

import (
	"context"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	ch "go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crosca/histogram"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var (
	playbackTime = 5 * time.Minute

	perfOutputFile            = "/var/log/perf_output.txt"
	videoSettingH264Dash60Fps = "H264 DASH 60 FPS"

	video = nodewith.Role(role.Video)

	// videoStreamingHistograms records histograms of Video Streaming performance comparisons.
	videoStreamingHistograms = []string{
		"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
		"EventLatency.KeyPressed.TotalLatency",
		"EventLatency.MousePressed.TotalLatency",
		"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
		"PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
		"PageLoad.InteractiveTiming.InputDelay3",
		"PageLoad.InteractiveTiming.TimeToNextPaint",
		"PageLoad.Experimental.NavigationTiming.NavigationStartToFirstResponseStart",
		"Graphics.Smoothness.Jank.AllSequences",
		"Graphics.Smoothness.Jank3.AllSequences",
	}
)

// playbackTimeVarString allow user to specify the duration of video playback
var playbackTimeVarString = testing.RegisterVarString(
	"crosca.VideoStreaming.playbackTime",
	"",
	"A integer value signifying the duration of video stream playback",
)

// showCountingDown indicate to user when to start power measuremnt manually
var manualPowerMeasurementVarString = testing.RegisterVarString(
	"crosca.VideoStreaming.manualpowermeasurement",
	"",
	"A BOOL value signifying if counting down of test should be shown to user",
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoStreaming,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A test case that plays the crosvideo use H264 decoding 60fps",
		Contacts:     []string{"chromeos-competitive-analysis@google.com", "williskung@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > crosca > Automation
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Params: []testing.Param{
			{
				Fixture: "loggedInToCUJUser",
				Timeout: 60 * time.Minute,
			},
			{
				Name:    "disable_arc",
				Fixture: "loggedInToCUJUserDisableARC",
				Timeout: 60 * time.Minute,
			},
		},
	})
}

// VideoStreaming measures the performance of critical user journeys for Video Streaming.
// This tast test is structured to create similar workload compared to the corresponding
// Windows python test.
func VideoStreaming(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	bTconn := tconn
	ui := uiauto.New(tconn)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	var conns []*chrome.Conn
	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")
		if len(conns) > 0 {
			for _, conn := range conns {
				conn.CloseTarget(ctx)
				conn.Close()
			}
		}
	}(cleanupCtx)

	if _, err := os.Stat(perfOutputFile); err == nil {
		s.Log("Delete old /var/log/perf_output.txt")
		err := os.Remove(perfOutputFile)
		if err != nil {
			s.Fatal("Failed to delete old /var/log/perf_output.txt: ", err)
		}
	}

	tabs, err := histogram.CreateHistogramTabs(ctx, cr, videoStreamingHistograms)
	if err != nil {
		s.Fatalf("Failed to create histogram tabs: %v", err)
	}
	defer func(ctx context.Context) {
		for _, tab := range tabs {
			if err := tab.Close(ctx); err != nil {
				s.Log("Failed to close histogram tab: ", err)
			}
		}
	}(cleanupCtx)

	histogramWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return strings.Contains(w.Title, "Histogram") })
	if err != nil {
		s.Fatal("Failed to find the histogram window: ", err)
	}
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{
		Mode: cujrecorder.Benchmark,
	})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)
	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	crosVideoConn, err := recorder.NewConn(ctx, cr.Browser(), "CrosVideo_Test", "https://crosvideo.appspot.com/?resolution=1080&loop=true", browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open cros video: ", err)
	}
	defer crosVideoConn.Close()
	crosVideoWindow, err := ash.FindOnlyWindow(ctx, tconn, ash.BrowserTitleMatch(browser.TypeAsh, "CrosVideo Test"))
	if err != nil {
		s.Fatal("Failed to find the CrosVideo window: ", err)
	}

	histogramsAfterCrosVideoLaunched, err := metrics.GetHistograms(ctx, tconn, videoStreamingHistograms)
	if err != nil {
		s.Log("Failed to open histogram pages: ", err)
	}
	s.Log("histogram snapshot after crosvideo launched: ", histogramsAfterCrosVideoLaunched)

	const expectedVolumePercent = 0
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create a volumeHelper")
	}
	originalVolume, err := vh.GetVolume(ctx)
	s.Logf("Set volumen from Original volume %d to zero", originalVolume)
	if err != nil {
		s.Fatal("Failed to get volume")
	}
	testing.ContextLogf(ctx, "Setting the audio volume to %d%% of the maximum volume. Current volume: %d", expectedVolumePercent, originalVolume)
	if err := vh.SetVolume(ctx, expectedVolumePercent); err != nil {
		s.Fatal("Failed to set volume to ", expectedVolumePercent)
	}
	defer vh.SetVolume(ctx, originalVolume)

	isBtEnabled, err := bluetooth.IsEnabled(ctx)
	if isBtEnabled {
		testing.ContextLog(ctx, "Start to disable bluetooth")
		if err := bluetooth.Disable(ctx); err != nil {
			s.Fatal("Failed to disable bluetooth: ", err)
		}
		testing.ContextLog(ctx, "Bluetooth disabled")
		defer func(ctx context.Context) {
			if err := bluetooth.Enable(ctx); err != nil {
				s.Fatal("Failed to connect bluetooth: ", err)
			}
		}(cleanupCtx)
	}

	if err := histogramWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate Histogram window: ", err)
	}
	if err := histogram.SwitchToMonitoringMode(ctx, ui, kb, tabs); err != nil {
		s.Fatal("Failed to switch to monitoring mode: ", err)
	}
	if err := crosVideoWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate the CrosVideo window: ", err)
	}
	pv := perf.NewValues()

	startVideoStreaming := func(ctx context.Context) error {
		playBackTimeInteger, err := readPlayBackTimeInteger(s)
		playbackTime = time.Duration(playBackTimeInteger) * time.Minute

		rows, err := ui.NodesInfo(ctx, nodewith.Role(role.Row))
		if err != nil {
			return err
		}
		for _, row := range rows {
			testing.ContextLogf(ctx, "%+v", row)
		}

		testManifestRow := nodewith.Role(role.Row).Nth(3)
		testManifestDropDown := nodewith.Name("H264 DASH 30 FPS").Role(role.Cell).FinalAncestor(testManifestRow)
		testManifestOption := nodewith.Name(videoSettingH264Dash60Fps).Role(role.ListBoxOption).FinalAncestor(testManifestDropDown)
		if err := uiauto.Combine("set the video",
			ui.LeftClick(testManifestDropDown),
			ui.LeftClick(testManifestOption),
		)(ctx); err != nil {
			s.Fatal("Failed to set the video: ", err)
		}

		// Maximized window instead of fullscreen such that mouse clicks can be injected.
		if err := ash.SetWindowStateAndWait(ctx, tconn, crosVideoWindow.ID, ash.WindowStateMaximized); err != nil {
			s.Log("Failed to ensure that the WebRTC Internals window is maximized: ", err)
		}

		histogramsBeforeVideoPlayback, err := metrics.GetHistograms(ctx, tconn, videoStreamingHistograms)
		if err != nil {
			s.Log("Failed to open histogram pages: ", err)
		}
		s.Log("histogram snapshot before video playback: ", histogramsBeforeVideoPlayback, "\n")

		// Capture the date on the device to check against the start time perf cmd
		// as a sanity check
		dateOutput, err := testexec.CommandContext(ctx, "date").Output()
		if err != nil {
			s.Log("Failed execute date command on the device")
		} else {
			// dateOutputStr := fmt.Sprintf("\n%s", string(dateOutput))
			s.Log("date command output: ", string(dateOutput))
		}

		showCountingDown := isManualPowerMeasurement()
		if showCountingDown == true {
			for i := 0; i < 3; i++ {
				s.Log("Play the video for started for ", playbackTime)
				// GoBigSleepLint: A brief delay to allow user to get ready to start power measuring
				// manually. Note that this test does not reply on battery fuel gauge to measure power
				// but instead using an external power meter. Therefore this loop was added to signal
				// user the start of video stream and mouse actions.
				testing.Sleep(ctx, 1*time.Second)
			}
		}

		if err := startVideoStreamingAndMouseAction(ctx, s, ui); err != nil {
			s.Fatal("Failed to run video streaming and mouse action: ", err)
		}

		if showCountingDown == true {
			for i := 0; i < 3; i++ {
				s.Log("completed video playback")
				// GoBigSleepLint: A brief delay to allow user to get ready to stop power measuring
				// manually.
				testing.Sleep(ctx, 1*time.Second)
			}
		}

		histogramsAfterVideoPlayback, err := metrics.GetHistograms(ctx, tconn, videoStreamingHistograms)
		if err != nil {
			s.Log("Failed to open histogram pages: ", err)
		}
		s.Log("histogram snapshot after video playback: ", histogramsAfterVideoPlayback)

		deltaHistograms, err := ch.DiffHistograms(histogramsBeforeVideoPlayback, histogramsAfterVideoPlayback)
		if err != nil {
			s.Log("Failed to diff histograms")
		}
		s.Log("delta histogram from during video playback: ", deltaHistograms, "\n")

		return kb.Accel(ctx, "Esc")
	}

	if err := recorder.Run(ctx, startVideoStreaming); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	decodedFrames := readDecodedFramesRow(ctx, s, 9, ui)
	pv.Set(perf.Metric{Name: "cros_video_decoded_frames", Unit: "count", Direction: perf.SmallerIsBetter}, decodedFrames)

	droppedFrames := readDecodedFramesRow(ctx, s, 10, ui)
	pv.Set(perf.Metric{Name: "cros_video_dropped_frames", Unit: "count", Direction: perf.SmallerIsBetter}, droppedFrames)

	if err := crosVideoWindow.CloseWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to close CrosVideo window: ", err)
	}
	if err := histogram.GenerateResults(ctx, ui, tabs, s.OutDir()); err != nil {
		s.Fatal("Failed to generate results: ", err)
	}
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
	if err := recorder.SaveHistograms(s.OutDir()); err != nil {
		s.Error("Failed to save histogram raw data")
	}
}

func enterFullScreen(ui *uiauto.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn) uiauto.Action {
	return uiauto.NamedCombine("press the Tab key three times to go to full screen",
		ui.LeftClick(video),
		kb.AccelAction("Tab"),
		kb.AccelAction("Tab"),
		kb.AccelAction("Tab"),
		kb.AccelAction("Tab"),
		kb.AccelAction("Enter"),
		func(ctx context.Context) error {
			return ash.WaitForFullScreen(ctx, tconn)
		},
	)
}

func isManualPowerMeasurement() bool {
	if strings.ToLower(manualPowerMeasurementVarString.Value()) != "true" {
		return true
	}
	return false
}

func readPlayBackTimeInteger(s *testing.State) (int, error) {
	var time int
	var err error
	if strings.ToLower(playbackTimeVarString.Value()) != "" {
		playbackTimeStr := strings.ToLower(playbackTimeVarString.Value())
		s.Log("crosca.VideoStreaming.playbackTime value: ", playbackTimeStr)
		time, err = strconv.Atoi(playbackTimeStr)
		if err != nil {
			s.Fatal("Failed to parse crosca.VideoStreaming.playbackTime: ", err)
		}
	}
	return time, nil
}

func readDecodedFramesRow(ctx context.Context, s *testing.State, row int, ui *uiauto.Context) float64 {
	decodedFramesRow := nodewith.Role(role.Row).Nth(row)
	decodedFramesCell := nodewith.Role(role.Cell).Nth(1).Ancestor(decodedFramesRow)
	info, err := ui.Info(ctx, decodedFramesCell)
	if err != nil {
		s.Fatal("Failed get decoded or dropped frames info: ", err)
	}
	decodedFrames, err := strconv.ParseFloat(info.Name, 64)
	if err != nil {
		s.Fatal("Failed get parse decoded or dropped frames: ", err)
	}

	return decodedFrames
}

func startVideoStreamingAndMouseAction(ctx context.Context, s *testing.State, ui *uiauto.Context) error {
	counter := 0

	perfCmd := testexec.CommandContext(ctx, "perf", "stat", "-a", "-e", "cycles,instructions,cache-references,cache-misses,bus-cycles,page-faults", "-o", "/var/log/perf_output.txt")
	if err := perfCmd.Start(); err != nil {
		perfCmd.DumpLog(ctx)
		s.Log("perf stat -e cycles,instructions -o /var/log/perf_output.txt")
	}

	for endTime := time.Now().Add(playbackTime); time.Now().Before(endTime); {

		st := time.Now()
		loadStream := nodewith.Name("Load stream").Role(role.Button)
		// Add a short and consistent delay to match corresponding operations in
		// Windows testing.
		if err := uiauto.Combine("Click load stream button",
			ui.LeftClick(loadStream),
			uiauto.Sleep(2*time.Second),
		)(ctx); err != nil {
			s.Fatal("Fail to click load stream button: ", err)
		}

		testManifestRow := nodewith.Role(role.Row).Nth(3)
		testManifestDropDown := nodewith.Name("H264 DASH 60 FPS").Role(role.Cell).FinalAncestor(testManifestRow)
		testManifestOption := nodewith.Name("H264 DASH 60 FPS").Role(role.ListBoxOption).FinalAncestor(testManifestDropDown)
		videoTracksRow := nodewith.Role(role.Row).Nth(6)
		cycleVideoCheckBox := nodewith.Role(role.CheckBox).Ancestor(videoTracksRow)
		for i := 1; i <= 20; i++ {
			// Generate some mouse click actions for mouse pressed measurement.
			// Add a short and consistent delay to match corresponding operations in
			// Windows testing.
			if i%2 == 0 {
				if err := uiauto.Combine("set the video",
					ui.LeftClick(testManifestDropDown),
					uiauto.Sleep(1*time.Second),
					ui.LeftClick(testManifestOption),
					uiauto.Sleep(1*time.Second),
				)(ctx); err != nil {
					s.Fatal("Failed to set the video: ", err)
				}
			}
			// Add a short and consistent delay to match corresponding operations in
			// Windows testing.
			if err := uiauto.Combine("Toggle Cycle Video Tracks Button",
				ui.LeftClick(cycleVideoCheckBox),
				uiauto.Sleep(2*time.Second),
			)(ctx); err != nil {
				s.Fatal("Fail to click Cycle Video Tracks button: ", err)
			}
		}

		dt := time.Now().Sub(st).Seconds()
		// Add a short and consistent delay to match corresponding operations in
		// Windows testing.
		if err := uiauto.Combine("press the Tab key to stop video playback",
			ui.LeftClick(nodewith.Role(role.Video)),
			uiauto.Sleep(1*time.Second),
		)(ctx); err != nil {
			s.Fatal("Failed to stop video playback: ", err)
		}

		decodedFramesRow := nodewith.Role(role.Row).Nth(8)
		decodedFramesCell := nodewith.Role(role.Cell).Nth(1).Ancestor(decodedFramesRow)
		info, err := ui.Info(ctx, decodedFramesCell)
		if err != nil {
			s.Fatal("Failed get decoded frames info: ", err)
		}
		decodedFrames, err := strconv.ParseFloat(info.Name, 64)
		if err != nil {
			s.Fatal("Failed get parse decoded frames: ", err)
		}

		droppedFramesRow := nodewith.Role(role.Row).Nth(9)
		droppedFramesCell := nodewith.Role(role.Cell).Nth(1).Ancestor(droppedFramesRow)
		info, err = ui.Info(ctx, droppedFramesCell)
		if err != nil {
			s.Fatal("Failed get dropped frames info: ", err)
		}
		droppedFrames, err := strconv.ParseFloat(info.Name, 64)
		if err != nil {
			s.Fatal("Failed get parse dropped frames: ", err)
		}

		s.Log("duration_sec: ", dt,
			" Stream info counter:", counter,
			" DecodeFrames: ", decodedFrames,
			" DroppedFrames: ", droppedFrames)

		counter = counter + 1
	}

	// Interrupt the cmd to stop recording perf (Abort is not used here as it will
	// destory recording)
	perfCmd.Signal(unix.SIGINT)
	err := perfCmd.Wait()
	if err != nil {
		s.Log("Failed closing perfCmd file")
	}
	// The signal is interrupt intentionally, so we check the wait status
	// instead of refusing the error.
	if ws, ok := testexec.GetWaitStatus(err); !ok || !ws.Signaled() || ws.Signal() != unix.SIGINT {
		s.Log("Failed to wait for the command to exit")
	}
	// Try to read the perf output. Perf data is debug only so failure
	// is not considered fatal
	s.Log("Read perf_output.txt")
	data, err := ioutil.ReadFile(perfOutputFile)
	if err != nil {
		s.Log("Failed reading perf_output.txt")
	}
	s.Log(string(data))

	return nil
}
