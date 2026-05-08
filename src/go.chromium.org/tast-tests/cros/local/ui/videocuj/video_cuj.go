// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package videocuj contains helper util and test code for VideoCUJ.
package videocuj

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio/crastests"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParam is the test parameters for VideoCUJ.
type TestParam struct {
	SystemTraceConfigPath string
	ExtraURLsToOpen       []string
}

// Run runs VideoCUJ by opening CrosVideo and playing the video at
// different resolutions and frame rates.
func Run(ctx context.Context, cr *chrome.Chrome, outDir string, testParam TestParam) (retErr error) {
	const (
		videoURL          = "https://crosvideo.appspot.com/?codec=%s&resolution=1080&loop=true"
		totalTestDuration = 10 * time.Minute
	)

	// The test will separately open each video format using each
	// of the following codecs.
	type codec struct {
		displayName   string
		crosVideoName string
	}
	var codecs = []codec{
		{
			displayName:   "h264_30",
			crosVideoName: "h264",
		},
		{
			displayName:   "h264_60",
			crosVideoName: "h264_60",
		},
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return errors.Wrap(err, "failed to capture device snapshot")
	}

	// Set up extra tabs requested by the caller.
	for _, url := range testParam.ExtraURLsToOpen {
		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			return errors.Wrapf(err, "failed to open url %s", url)
		}
		defer conn.Close()
	}

	// Set up an about:blank page, so that we can use the given
	// connection to navigate to CrosVideo within the recorder.
	videoConn, err := cr.NewConn(ctx, chrome.BlankURL)
	if err != nil {
		return errors.Wrap(err, "failed to set up Chrome")
	}
	defer videoConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API connection")
	}

	recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(); err != nil {
		return errors.Wrap(err, "failed to add common metrics to the recorder")
	}

	// Add an empty screenshot recorder. We will manually take screenshots
	// after every section of the test.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder: ", err)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	testing.ContextLog(ctx, "Is in tablet-mode: ", inTabletMode)
	if err != nil {
		return errors.Wrap(err, "failed to detect if device is in tablet-mode or not")
	}

	var pc pointer.Context
	if inTabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to create a touch controller")
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a keyboard")
	}
	defer kw.Close(ctx)

	ui := uiauto.New(tconn)

	// Mute the whole device, to prevent disturbing the lab while
	// fiddling with the volume slider during the test.
	if err := crastests.Mute(ctx); err != nil {
		return errors.Wrap(err, "failed to mute audio")
	}
	defer crastests.Unmute(closeCtx)

	tabChecker, err := cuj.NewTabCrashChecker(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to create TabCrashChecker")
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")

	videoWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get active window")
	}

	// videoEvalAction returns an action.Action that executes the given
	// |script| with the connection to the video tab.
	videoEvalAction := func(script string) action.Action {
		return func(ctx context.Context) error { return videoConn.Eval(ctx, script, nil) }
	}

	// setVideoFormats contains information needed to set up and clean up
	// each desired video format.
	setVideoFormats := []struct {
		name    string
		setUp   action.Action
		cleanUp action.Action
	}{
		{
			name:    "Normal",
			setUp:   func(ctx context.Context) error { return nil },
			cleanUp: func(ctx context.Context) error { return nil },
		},
		{
			name:    "PIP",
			setUp:   videoEvalAction("video.requestPictureInPicture().then(() => {})"),
			cleanUp: videoEvalAction("document.exitPictureInPicture().then(() =>{})"),
		},
		{
			name: "Fullscreen",
			setUp: action.Combine(
				"enter fullscreen",
				videoEvalAction("video.requestFullscreen().then(() => {})"),
				ash.WaitForFullscreenConditionWithTitle(tconn, "CrosVideo", true, time.Minute),
			),
			cleanUp: action.Combine(
				"exit fullscreen",
				videoEvalAction("exitFullscreen()"),
				ash.WaitForFullscreenConditionWithTitle(tconn, "CrosVideo", false, time.Minute),
			),
		},
	}

	// Get a list of Ash and browser metrics we want to take snapshots after
	// every phase of the test (each codec/video format pairing).
	ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()

	// Keep track of the start time for recorder.Run, so we can ensure that
	// the test runs for the same amount of time on every device.
	runStart := time.Now()

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		const videoPrefix = "CrosVideo"
		var totalDroppedFrames int
		var totalDecodedFrames int
		// Record a trace of the first iteration of the test.
		shouldRecordTrace := true
		for _, codec := range codecs {
			// Open the video with the given codec.
			if err := videoConn.Navigate(ctx, fmt.Sprintf(videoURL, codec.crosVideoName)); err != nil {
				return errors.Wrapf(err, "failed to navigate to %q", videoURL)
			}

			if err := webutil.WaitForQuiescence(ctx, videoConn, 10*time.Second); err != nil {
				testing.ContextLogf(ctx, "Failed to wait for the tab %s to quiesce", videoURL)
			}

			crosVideoWebArea := nodewith.NameContaining("CrosVideo").Role(role.RootWebArea)
			if err := webutil.ReloadIfSiteUnreachable(ui, videoConn, crosVideoWebArea)(ctx); err != nil {
				return errors.Wrap(err, "failed to reload the page")
			}

			// Switch between each video format (Normal, PIP, Fullscreen).
			for _, format := range setVideoFormats {
				if err := tabChecker.Check(ctx); err != nil {
					return errors.Wrap(err, "tab renderer crashed")
				}

				// Properly focus on the window. Click on the window a few
				// times to try to get the window focused. This can't be done
				// through the UI tree, or else calling Javascript functions on
				// the video object fail with:
				// "TypeError: Permissions check failed".
				if err := pc.ClickAt(videoWindow.BoundsInRoot.CenterPoint())(ctx); err != nil {
					return errors.Wrap(err, "failed to focus the video window")
				}

				for _, direction := range []string{"Down", "Up"} {
					if err := inputsimulations.RepeatKeyPress(ctx, kw, direction, 500*time.Millisecond, 3); err != nil {
						return errors.Wrapf(err, "failed to repeatedly press the %s arrow key", direction)
					}
				}

				if err := ui.WithTimeout(3*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to wait until no event after focusing window: ", err)
				}

				video := nodewith.Role(role.Video)
				if err := ui.WaitUntilExists(video)(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for the video")
				}

				// Ensure the video is playing before we set up the video format.
				if err := videoConn.Eval(ctx, "video.play()", nil); err != nil {
					return errors.Wrap(err, "failed to play video")
				}

				// Properly activate the desired video format, such as PIP
				// or fullscreen.
				if err := format.setUp(ctx); err != nil {
					return errors.Wrapf(err, "failed to set up %q state", format.name)
				}

				if err = ash.WaitWindowFinishAnimating(ctx, tconn, videoWindow.ID); err != nil {
					return errors.Wrap(err, "failed to wait for video window to finish animating")
				}

				if err := ui.WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to wait until no event: ", err)
				}

				// Start watching, and record the initial decoded frames and
				// dropped frame information to compare against after this
				// section of the test completes.
				recorder.Annotate(ctx, fmt.Sprintf("Start_watching_%s_%s", format.name, codec.displayName))
				prevDroppedFrames, prevDecodedFrames, err := getFrameData(ctx, videoConn)
				if err != nil {
					return errors.Wrap(err, "failed to get initial video frame data")
				}

				stopSnapshot, err := recorder.StartSnapshot(ctx, fmt.Sprintf("%s.%s.%s", videoPrefix, format.name, codec.displayName), ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start recording a snapshot")
				}

				// See go/trace-in-cuj-tests about rules for tracing.
				if shouldRecordTrace {
					if err := recorder.StartTracing(ctx, outDir, testParam.SystemTraceConfigPath); err != nil {
						return errors.Wrap(err, "failed to start tracing")
					}
				}

				// Consider a "cycle" to be 90 seconds. This cycle should
				// include watching the video, interacting with the video
				// player, and interacting with the Ash UI.
				const cycleTotalTime = 90 * time.Second
				cycleTimeStart := time.Now()

				// Watch the video for 30 seconds, to ensure we are getting
				// meaningful graphical performance data. If in clamshell
				// mode, move the mouse to get mouse input latency.
				initialSleepDuration := 30 * time.Second
				if !inTabletMode {
					if err := inputsimulations.MoveMouseFor(ctx, tconn, initialSleepDuration); err != nil {
						return errors.Wrap(err, "failed to simulate mouse movement")
					}
				} else {
					// GoBigSleepLint: Watch the video for 30 seconds to
					// collect graphical metrics.
					if err := testing.Sleep(ctx, initialSleepDuration); err != nil {
						return errors.Wrap(err, "failed to sleep")
					}
				}

				// Interact with the Ash UI. Explicitly only do an overview mode
				// action, because when the video is in fullscreen, the system
				// tray is not visible.
				if err := inputsimulations.DoOverviewWorkflow(ctx, tconn, pc); err != nil {
					return errors.Wrap(err, "failed to interact with a window in overview mode")
				}

				// Collect input latency metrics by toggling and dragging
				// the volume slider. Move the mouse in clamshell/tablet for
				// consistency in making the media control buttons appear.
				// Match the volume button by regex to account for both mute
				// and unmute buttons.
				volumeButton := nodewith.NameRegex(regexp.MustCompile("mute$")).Role(role.Button)
				volumeSlider := nodewith.Name("volume").Role(role.Slider)

				// Ensure the video is paused to get the location of volume button.
				if err := videoConn.Eval(ctx, "video.pause()", nil); err != nil {
					return errors.Wrap(err, "failed to pause video")
				}
				if err := action.Combine(
					"open volume slider toggle",
					ui.MakeVisible(video),
					ui.WaitUntilExists(volumeButton),
					// The volume button is frequently offscreen. ui.MakeVisible
					// scrolls the page until the button is visible on screen.
					ui.MakeVisible(volumeButton),
					ui.EnsureFocused(volumeButton),
				)(ctx); err != nil {
					return err
				}
				muteLocation, err := ui.Location(ctx, volumeButton)
				if err != nil {
					return errors.Wrap(err, "failed to get the mute button location")
				}
				// Ensure the video is playing after we get the location of volume button.
				if err := videoConn.Eval(ctx, "video.play()", nil); err != nil {
					return errors.Wrap(err, "failed to play video")
				}

				// Tap the mute button to collect mouse/touch input latency.
				if err := uiauto.Repeat(10, action.Combine(
					"toggle mute button repeatedly",
					pc.ClickAt(muteLocation.CenterPoint()),
					action.Sleep(500*time.Millisecond),
				))(ctx); err != nil {
					return err
				}

				// Wait for any leftover animations to complete .
				if err := ui.WithTimeout(5*time.Second).WaitUntilNoEvent(volumeSlider, event.LocationChanged)(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to wait for volume slider to stabilize: ", err)
				}

				// Press the down and up arrow key 15 times, with 100 milliseconds
				// in between each press.
				for _, direction := range []string{"Down", "Up"} {
					if err := inputsimulations.RepeatKeyPress(ctx, kw, direction, 100*time.Millisecond, 15); err != nil {
						return errors.Wrapf(err, "failed to repeatedly press the %s arrow key", direction)
					}
				}

				// If in clamshell mode, move the mouse to the center of the video.
				// Thus, when we let the video run, the menu items should properly
				// disappear.
				if !inTabletMode {
					if err := ui.MouseMoveTo(video, 500*time.Millisecond)(ctx); err != nil {
						return errors.Wrap(err, "failed to move the mouse to the center of the video")
					}
				}

				// GoBigSleepLint: Sleep until we hit the total cycle length.
				// This way, each video format plays for the exact same amount
				// of time.
				if err := testing.Sleep(ctx, cycleTotalTime-time.Since(cycleTimeStart)); err != nil {
					return errors.Wrap(err, "failed to sleep")
				}

				if shouldRecordTrace {
					if err := recorder.StopTracing(ctx); err != nil {
						return errors.Wrap(err, "failed to stop tracing")
					}
					shouldRecordTrace = false
				}

				// Before cleaning up, track how many frames were dropped in
				// this section.
				droppedFrames, decodedFrames, err := getFrameData(ctx, videoConn)
				if err != nil {
					return errors.Wrapf(err, "failed to get video frame data after finishing %s %s", format.name, codec.displayName)
				}

				sectionDecodedFrames := decodedFrames - prevDecodedFrames
				if sectionDecodedFrames == 0 {
					return errors.Errorf("got 0 decoded frames for %s %s", format.name, codec.displayName)
				}

				sectionDroppedFrames := droppedFrames - prevDroppedFrames

				pv.Set(perf.Metric{
					Name:      fmt.Sprintf("%s.%s.%s.DroppedFrames", videoPrefix, format.name, codec.displayName),
					Unit:      "frames",
					Direction: perf.SmallerIsBetter,
				}, float64(sectionDroppedFrames))
				pv.Set(perf.Metric{
					Name:      fmt.Sprintf("%s.%s.%s.DecodedFrames", videoPrefix, format.name, codec.displayName),
					Unit:      "frames",
					Direction: perf.BiggerIsBetter,
				}, float64(sectionDecodedFrames))
				pv.Set(perf.Metric{
					Name:      fmt.Sprintf("%s.%s.%s.PercentDroppedFrames", videoPrefix, format.name, codec.displayName),
					Unit:      "percent",
					Direction: perf.SmallerIsBetter,
				}, float64(sectionDroppedFrames)/float64(sectionDecodedFrames))

				recorder.CustomScreenshot(ctx)

				if err := format.cleanUp(ctx); err != nil {
					return errors.Wrapf(err, "failed to clean up %q state", format.name)
				}
				recorder.Annotate(ctx, fmt.Sprintf("Finished_watching_%s_%s", format.name, codec.displayName))

				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop recording a snapshot")
				}
			}

			// Record decoded/dropped frames for each codec.
			droppedFrames, decodedFrames, err := getFrameData(ctx, videoConn)
			if err != nil {
				return errors.Wrap(err, "failed to get video frame data at end of test")
			}
			totalDroppedFrames += droppedFrames
			totalDecodedFrames += decodedFrames

			pv.Set(perf.Metric{
				Name:      fmt.Sprintf("%s.%s.DroppedFrames", videoPrefix, codec.displayName),
				Unit:      "frames",
				Direction: perf.SmallerIsBetter,
			}, float64(droppedFrames))
			pv.Set(perf.Metric{
				Name:      fmt.Sprintf("%s.%s.DecodedFrames", videoPrefix, codec.displayName),
				Unit:      "frames",
				Direction: perf.BiggerIsBetter,
			}, float64(decodedFrames))
			pv.Set(perf.Metric{
				Name:      fmt.Sprintf("%s.%s.PercentDroppedFrames", videoPrefix, codec.displayName),
				Unit:      "percent",
				Direction: perf.SmallerIsBetter,
			}, float64(droppedFrames)/float64(decodedFrames))

		}
		// Video should be in the normal state. Let the test run until 10
		// minutes have passed, to standardize how long recorder.Run takes.
		timeLeft := totalTestDuration - time.Since(runStart)
		if timeLeft > 0 {
			testing.ContextLogf(ctx, "Sleeping for %s to close out the test", timeLeft)
			// GoBigSleepLint: Run until total test time has passed.
			if err := testing.Sleep(ctx, timeLeft); err != nil {
				return errors.Wrap(err, "failed to sleep to close out the test")
			}
		}

		// Record dropped/decoded frames across the entire test run.
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s.DroppedFrames", videoPrefix),
			Unit:      "frames",
			Direction: perf.SmallerIsBetter,
		}, float64(totalDroppedFrames))
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s.DecodedFrames", videoPrefix),
			Unit:      "frames",
			Direction: perf.BiggerIsBetter,
		}, float64(totalDecodedFrames))
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s.PercentDroppedFrames", videoPrefix),
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, float64(totalDroppedFrames)/float64(totalDecodedFrames))

		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to conduct the recorder task")
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return errors.Wrap(err, "failed to report")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}
	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store values")
	}
	return nil
}

// getFrameData reads the dropped frames and decoded frames from a given
// CrosVideo connection.
func getFrameData(ctx context.Context, crosVideoConn *chrome.Conn) (int, int, error) {
	var droppedFrames int
	if err := crosVideoConn.Eval(ctx, "getDroppedFrames()", &droppedFrames); err != nil {
		return 0, 0, errors.Wrap(err, "failed to get dropped frames from CrosVideo")
	}

	var decodedFrames int
	if err := crosVideoConn.Eval(ctx, "getDecodedFrames()", &decodedFrames); err != nil {
		return 0, 0, errors.Wrap(err, "failed to get decoded frames from CrosVideo")
	}

	return droppedFrames, decodedFrames, nil
}
