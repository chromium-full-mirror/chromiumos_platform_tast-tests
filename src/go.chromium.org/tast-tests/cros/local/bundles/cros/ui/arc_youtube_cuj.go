// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcYoutubeCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the performance of critical user journey for the YouTube ARC app",
		Contacts:     []string{"cros-sw-perf@google.com", "ramsaroop@chromium.org"},
		BugComponent: "b:1045832",
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Fixture:      "loggedInToCUJUserARCSupported",
		Timeout:      20 * time.Minute,
	})
}

// ArcYoutubeCUJ measures the performance of critical user journey for the YouTube ARC app.
func ArcYoutubeCUJ(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(cuj.FixtureData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	closedUIDevice := false
	closeUIDevice := func(ctx context.Context) {
		if closedUIDevice {
			return
		}
		closedUIDevice = true
		if err := d.Close(ctx); err != nil {
			s.Log("Failed closing UI Automator: ", err)
		}
	}
	defer closeUIDevice(cleanupCtx)

	const ytAppPkgName = "com.google.android.youtube"
	if err := playstore.InstallOrUpdateAppAndClose(ctx, tconn, a, d, ytAppPkgName, &playstore.Options{}); err != nil {
		s.Fatal("Failed to install ARC++ YouTube app: ", err)
	}

	act, err := arc.NewActivity(a, ytAppPkgName, "com.google.android.apps.youtube.app.WatchWhileActivity")
	if err != nil {
		s.Fatal("Failed to create ARC++ YouTube app activity: ", err)
	}
	defer act.Close(ctx)

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, a, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	// Add an empty screenshot recorder.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		// Launch the ARC YouTube app.
		if err := act.Start(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to start ARC++ YouTube app")
		}
		defer act.Stop(cleanupCtx, tconn)
		// Dump the ARC UI hierarchy before closing the ARC YouTube app.
		defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), func() bool { return retErr != nil })
		// Close the ARC UI automator before dumping the UI hierarchy. Then the
		// hierarchy dump will not be ruined by UI automator errors like status 137.
		defer closeUIDevice(cleanupCtx)
		// Take a screenshot before closing the ARC YouTube app.
		defer recorder.CustomScreenshot(cleanupCtx)

		if err := youtube.DismissPrompts(d)(ctx); err != nil {
			s.Fatal("Failed to dismiss prompts: ", err)
		}

		const uiTimeout = 15 * time.Second
		clickSearchButton := func(ctx context.Context) error {
			navigateUpButton := d.Object(
				ui.ClassName("android.widget.ImageButton"),
				ui.Description("Navigate up"),
				ui.PackageName(ytAppPkgName),
			)
			// If the app is currently not on the home page, go back to it.
			if err := cuj.ClickIfExist(navigateUpButton, 5*time.Second)(ctx); err != nil {
				return errors.Wrap(err, "failed to click navigate up button")
			}

			searchImageViewButton := d.Object(
				ui.ClassName("android.widget.ImageView"),
				ui.Description("Search"),
				ui.PackageName(ytAppPkgName),
			)
			searchWidgetButton := d.Object(
				ui.ClassName("android.widget.Button"),
				ui.Description("Search"),
				ui.PackageName(ytAppPkgName),
			)
			// There might be two different ARC dump hierarchies that affect how nodes are captured.
			searchButton, err := cuj.FindAnyExists(ctx, uiTimeout, searchImageViewButton, searchWidgetButton)
			if err != nil {
				return errors.Wrap(err, "failed to find Search button")
			}
			return searchButton.Click(ctx)
		}

		// Put 862r3XS2YB0 in the search box for searching https://www.youtube.com/watch?v=862r3XS2YB0.
		const testVideoID = "862r3XS2YB0"
		searchVideo := func(videoID string) uiauto.Action {
			return func(ctx context.Context) error {
				searchQueryField := d.Object(
					ui.ID(ytAppPkgName+":id/search_edit_text"),
					ui.ClassName("android.widget.EditText"),
					ui.PackageName(ytAppPkgName),
				)
				if err := searchQueryField.WaitForExists(ctx, uiTimeout); err != nil {
					return errors.Wrap(err, "failed to wait for Search query field")
				}
				if err := searchQueryField.SetText(ctx, videoID); err != nil {
					return errors.Wrap(err, "failed to set search query")
				}
				// Press Enter to search.
				return d.PressKeyCode(ctx, ui.KEYCODE_ENTER, 0)
			}
		}

		const retryTimes = 3
		video := d.Object(
			ui.ClassName("android.view.ViewGroup"),
			ui.DescriptionContains("Google I/O 2016 - Keynote"),
			ui.PackageName(ytAppPkgName),
		)
		if err := uiauto.Retry(retryTimes, uiauto.NamedCombine("search video",
			clickSearchButton,
			searchVideo(testVideoID),
			cuj.FindAndClick(video, uiTimeout),
		))(ctx); err != nil {
			return errors.Wrap(err, "failed to search video")
		}

		// Wait for the seek bar.
		seekBar := d.Object(
			ui.ClassName("android.widget.SeekBar"),
			ui.PackageName(ytAppPkgName),
		)
		if err := seekBar.WaitForExists(ctx, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for seek bar")
		}

		// Wait for the video to load (just enough that it can start playing).
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			videoPosition, err := seekBar.GetContentDescription(ctx)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get video position from seek bar"))
			}

			if videoPosition == "0 minutes 0 seconds of 0 minutes 0 seconds" {
				return errors.New("video is still loading")
			}
			// Log the position along the timeline of video playback. Video playback starts from the
			// position where the user left off if they were watching the same video in the past.
			s.Log("Video is starting from: ", videoPosition)
			recorder.Annotate(ctx, "Video_loaded")
			return nil
		}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
			return errors.Wrap(err, "failed to wait for video to load")
		}

		// Wait for the recommended videos section to load.
		recommendedVideosLandmark := d.Object(
			ui.ClassName("android.widget.ImageView"),
			ui.Description("Action menu"),
			ui.PackageName(ytAppPkgName),
		)
		if err := recommendedVideosLandmark.WaitForExists(ctx, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for recommended videos section to load")
		}

		// Get the position along the timeline of video playback.
		videoPosition, err := seekBar.GetContentDescription(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get video position from seek bar")
		}

		// Log the position along the timeline of video playback.
		s.Log("Initial video position (after waiting for everything to load): ", videoPosition)

		switchQuality := func(ctx context.Context) error {
			// Make sure the video is 60fps.
			if err := d.PressKeyCode(ctx, ui.KEYCODE_ESCAPE, 0); err != nil {
				return errors.Wrap(err, "failed to press ESC")
			}
			optionsView := d.Object(
				ui.ClassName("android.widget.ImageView"),
				ui.Description("More options"),
				ui.PackageName(ytAppPkgName),
			)
			if err := optionsView.Click(ctx); err != nil {
				return errors.Wrap(err, "failed to click the options view")
			}
			// TODO(b/277108157): Assume the first button is `Quality` button because it can not
			// find the `Quality` button with UI Automator selector.
			if err := d.PressKeyCode(ctx, ui.KEYCODE_TAB, 0); err != nil {
				return errors.Wrap(err, "failed to press TAB")
			}
			// Capture screenshot of the options list for further debugging.
			recorder.CustomScreenshot(ctx)
			if err := d.PressKeyCode(ctx, ui.KEYCODE_ENTER, 0); err != nil {
				return errors.Wrap(err, "failed to press ENTER")
			}
			// Sometimes clicking `Quality` button will show "Quality unavailable"
			// message and changing quality will not be allowed.
			qualityUnavailableText := d.Object(
				ui.ClassName("android.widget.TextView"),
				ui.Text("Quality unavailable"),
				ui.PackageName(ytAppPkgName),
			)
			if err := qualityUnavailableText.WaitForExists(ctx, 3*time.Second); err == nil {
				return errors.New(`"Quality unavailable" message is shown, cannot change quality`)
			}

			advanceOption := d.Object(
				ui.ClassName("android.view.ViewGroup"),
				// The fourth button is "Advance".
				ui.Index(3),
				ui.PackageName(ytAppPkgName),
				ui.Clickable(true),
			)
			advanceView := d.Object(
				ui.ClassName("android.widget.TextView"),
				ui.TextContains("Advance"),
				ui.PackageName(ytAppPkgName),
			)
			// There might be two different ARC dump hierarchies that affect
			// how nodes are captured.
			advanceButton, err := cuj.FindAnyExists(ctx, uiTimeout, advanceOption, advanceView)
			if err != nil {
				return errors.Wrap(err, "failed to find Advance button")
			}
			if err := advanceButton.Click(ctx); err != nil {
				return errors.Wrap(err, "failed to click Advance button")
			}

			return youtube.ClickQualityOption(d, cr, s.OutDir(), youtube.Quality1080P60)(ctx)
		}
		if err := uiauto.Retry(retryTimes, uiauto.NamedAction("switch quality", switchQuality))(ctx); err != nil {
			return errors.Wrap(err, "failed to switch quality")
		}

		recorder.Annotate(ctx, "Start_watching_video")

		var traceRecorded bool
		for endTime := time.Now().Add(10 * time.Minute); time.Now().Before(endTime); {
			if !traceRecorded {
				// See go/trace-in-cuj-tests about rules for tracing.
				if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
			}
			const verificationInterval = 30 * time.Second
			// GoBigSleepLint: Sleep 30 seconds to let the video play before checking the state of the video.
			if err := testing.Sleep(ctx, verificationInterval); err != nil {
				return errors.Wrapf(err, "failed to wait %s", verificationInterval)
			}
			if !traceRecorded {
				if err := recorder.StopTracing(ctx); err != nil {
					return errors.Wrap(err, "failed to stop tracing")
				}
				traceRecorded = true
			}

			// Get the current position along the timeline of video playback, and
			// verify that it has changed (so the video is actually playing).
			updatedPosition, err := seekBar.GetContentDescription(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get video position from seek bar")
			}
			if updatedPosition == videoPosition {
				return errors.Errorf("video has not progressed for %s", verificationInterval)
			}
			videoPosition = updatedPosition
			s.Log("Video position: ", videoPosition)

			// Verify that the recommended videos section is still loaded.
			if err := recommendedVideosLandmark.Exists(ctx); err != nil {
				return errors.Wrap(err, "failed to verify that the recommended videos section is still loaded")
			}
		}
		recorder.Annotate(ctx, "Stop_watching_video")

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the performance measurement: ", err)
	}

	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the performance data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save the performance data: ", err)
	}
}
