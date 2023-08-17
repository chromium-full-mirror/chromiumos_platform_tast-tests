// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package videocuj contains helper util and test code for VideoCUJ.
package videocuj

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Run runs VideoCUJ by opening CrosVideo and playing the video at
// different resolutions and frame rates.
func Run(ctx context.Context, s *testing.State) {
	const (
		videoURL          = "http://crosvideo.appspot.com/?codec=%s&resolution=1080&loop=true"
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
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Set up an about:blank page, so that we can use the given
	// connection to navigate to CrosVideo within the recorder.
	videoConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to set up Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer videoConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API connection: ", err)
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to the recorder: ", err)
	}

	// Add an empty screenshot recorder. We will manually take screenshots
	// after every section of the test.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	// TODO (b/277656113): Determine whether or not to generalize GPU usage
	// collection to all TPS tests after comparing the performance.
	if strings.Contains(s.TestName(), "gpuusage") {
		gpuUsageTimeline, err := perf.NewTimeline(ctx, []perf.TimelineDatasource{
			power.NewGPUUsageDataSource()}, perf.Interval(5*time.Second))
		if err != nil {
			s.Fatal("Failed to create GPU usage timeline: ", err)
		}
		recorder.AddPerfTimelines(gpuUsageTimeline)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	s.Logf("Is in tablet-mode: %t", inTabletMode)
	if err != nil {
		s.Fatal("Failed to detect if device is in tablet-mode or not: ", err)
	}

	var pc pointer.Context
	if inTabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	ui := uiauto.New(tconn)

	// Mute the whole device, to prevent disturbing the lab while
	// fiddling with the volume slider during the test.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute audio: ", err)
	}
	defer crastestclient.Unmute(closeCtx)

	tabChecker, err := cuj.NewTabCrashChecker(ctx, bTconn)
	if err != nil {
		s.Fatal("Failed to create TabCrashChecker: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// The battery saver alert would inactivate the current window.
	// Dismiss the alert if it pops up.
	if strings.Contains(s.TestName(), "battery_saver") {
		if err := cuj.DismissBatterySaverPrompt(tconn)(ctx); err != nil {
			s.Fatal("Failed to dismiss battery saver prompt: ", err)
		}
	}

	videoWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
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
		for _, codec := range codecs {
			// Open the video with the given codec.
			if err := videoConn.Navigate(ctx, fmt.Sprintf(videoURL, codec.crosVideoName)); err != nil {
				return errors.Wrapf(err, "failed to navigate to %q", videoURL)
			}

			if err := webutil.WaitForQuiescence(ctx, videoConn, 10*time.Second); err != nil {
				s.Logf("Failed to wait for the tab %s to quiesce", videoURL)
			}

			// Switch between each video format (Normal, PIP, Fullscreen).
			for _, format := range setVideoFormats {
				if err := tabChecker.Check(ctx); err != nil {
					s.Fatal("Tab renderer crashed: ", err)
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
					s.Log("Failed to wait until no event after focusing window: ", err)
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
					s.Log("Failed to wait until no event: ", err)
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
				video := nodewith.Role(role.Video)

				// Match the volume button by regex to account for both mute
				// and unmute buttons.
				volumeButton := nodewith.NameRegex(regexp.MustCompile("mute$")).Role(role.Button)
				volumeSlider := nodewith.Name("volume").Role(role.Slider)
				if err := action.Combine(
					"open volume slider toggle",

					// Move the mouse to the left of the screen, then back to
					// the center of the video. By moving the mouse to the
					// center of the video, the media controls become visible.
					mouse.Move(tconn, info.Bounds.LeftCenter(), 200*time.Millisecond),
					ui.MouseMoveTo(video, 200*time.Millisecond),
					ui.WaitUntilExists(volumeButton),

					// The volume button is frequently offscreen. ui.MakeVisible
					// scrolls the page until the button is visible on screen.
					ui.MakeVisible(volumeButton),

					// Move the mouse to the button and wait until the volume
					// slider exists, and the animations are complete.
					ui.MouseMoveTo(volumeButton, 200*time.Millisecond),
					ui.WaitUntilExists(volumeSlider),
					ui.WaitForLocation(volumeSlider),
				)(ctx); err != nil {
					return err
				}

				muteLocation, err := ui.Location(ctx, volumeButton)
				if err != nil {
					return errors.Wrap(err, "failed to get the mute button location")
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
					s.Log("Failed to wait for volume slider to stabilize: ", err)
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
			s.Logf("Sleeping for %s to close out the test", timeLeft)
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
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
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
