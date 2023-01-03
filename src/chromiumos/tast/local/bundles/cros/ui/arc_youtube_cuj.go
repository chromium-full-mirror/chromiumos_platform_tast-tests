// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcYoutubeCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the performance of critical user journey for the YouTube ARC app",
		Contacts:     []string{"chromeos-perfmetrics-eng@google.com", "amusbach@chromium.org"},
		BugComponent: "b:1045832",
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Fixture:      "loggedInToCUJUser",
		Timeout:      20 * time.Minute,
	})
}

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
	defer act.Close()

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, a, cujrecorder.RecorderOptions{StopMetricsBeforeTracing: true})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	recorder.EnableTracing(s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile))

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

		// Click the Search icon.
		searchIcon := d.Object(
			ui.ClassName("android.widget.ImageView"),
			ui.Description("Search"),
			ui.PackageName(ytAppPkgName),
		)
		const uiTimeout = 15 * time.Second
		if err := searchIcon.WaitForExists(ctx, uiTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for Search icon")
		}
		if err := searchIcon.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click Search")
		}

		// Put 862r3XS2YB0 in the search box, because we want this video: https://www.youtube.com/watch?v=862r3XS2YB0
		searchQueryField := d.Object(
			ui.Text("Search YouTube"),
			ui.ClassName("android.widget.EditText"),
			ui.PackageName(ytAppPkgName),
		)
		if err := searchQueryField.WaitForExists(ctx, uiTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for Search query field")
		}
		if err := searchQueryField.SetText(ctx, "862r3XS2YB0"); err != nil {
			return errors.Wrap(err, "failed to set search query")
		}

		// Press Enter to search.
		if err := d.PressKeyCode(ctx, ui.KEYCODE_ENTER, 0); err != nil {
			return errors.Wrap(err, "failed to press Enter")
		}

		// Click the desired video.
		video := d.Object(
			ui.ClassName("android.view.ViewGroup"),
			ui.DescriptionMatches("Google I/O 2016 - Keynote - 1 hour, 54 minutes - Go to channel - Google Developers .+ - play video"),
			ui.PackageName(ytAppPkgName),
		)
		if err := video.WaitForExists(ctx, uiTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for search results")
		}
		if err := video.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click for video")
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

		// Monitor video playback.
		for endTime := time.Now().Add(10 * time.Minute); time.Now().Before(endTime); {
			const verificationInterval = 30 * time.Second
			if err := testing.Sleep(ctx, verificationInterval); err != nil {
				return errors.Wrapf(err, "failed to wait %s", verificationInterval)
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

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the performance measurement: ", err)
	}

	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the performance data: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save the performance data: ", err)
	}
}
