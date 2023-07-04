// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakehtml"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraEffectsChrome,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify camera effects using screen test",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Attr: []string{
			"group:camera_dependent",
			"group:video_conference",
			"video_conference_per_build",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Timeout:      5 * time.Minute,
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
		},
		Vars: screenshot.ScreenDiffVars,
		SearchFlags: []*testing.StringPair{
			{
				// Enable background blur.
				Key:   "feature_id",
				Value: "screenplay-895a7022-c19e-42b9-ad7c-c3e4a0693210",
			},
			{
				// Lighting improvement on visible person.
				Key:   "feature_id",
				Value: "screenplay-b36fac8e-b0d6-4ba4-9c1d-fa172326816d",
			},
			{
				// Blur and lightning improvement.
				Key:   "feature_id",
				Value: "screenplay-1f125a69-4a47-4156-8cb5-97bc6244016b",
			},
		},
		Params: []testing.Param{
			{
				Fixture: fixture.LoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:    "lacros",
				Fixture: fixture.LoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
		},
	})
}

func CameraEffectsChrome(ctx context.Context, s *testing.State) {
	// Shorten context to allow for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Open video on simple javascript browser.
	testing.ContextLog(ctx, "Opening Simple Meeting")
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()

	url := srv.URL + fakehtml.PageURL
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, url)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer conn.CloseTarget(cleanupCtx)
	defer conn.Close()
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	fakeHTMLUI := fakehtml.NewUI(tconn)
	if err := fakeHTMLUI.MayBeAllowCameraAccess(ctx); err != nil {
		s.Fatal("Failed to allow camera access: ", err)
	}

	// Maximize VC app window in clamshell mode to reduce resolution noises on different devices.
	// Skip Tablet mode as app is full screen by default.
	if inTabletMode, err := ash.TabletModeEnabled(ctx, tconn); err != nil {
		s.Fatal("Failed to get tablet-mode status: ", err)
	} else if !inTabletMode {
		if _, err := ash.MaximizeWindowTitleContains(ctx, tconn, fakehtml.PageTitle); err != nil {
			s.Fatal("Failed to maximize Camera window: ", err)
		}
	}

	vcTray := vctray.New(ctx, tconn)

	// Run subtests to verify video effects are correctly applied.
	// Note: Golden images can be found at https://cros-tast-gold.skia.org/list?corpus=videoconferencing.
	subTests := []struct {
		name               string
		backgroundBlur     vctray.BackgroundBlurLevel
		portraitRelighting bool
	}{
		{
			name:               "backgroundblur_off_portraitrelighting_off",
			backgroundBlur:     vctray.BackgroundBlurOff,
			portraitRelighting: false,
		},
		{
			name:               "backgroundblur_light_portraitrelighting_off",
			backgroundBlur:     vctray.BackgroundBlurLight,
			portraitRelighting: false,
		},
		{
			name:               "backgroundblur_full_portraitrelighting_off",
			backgroundBlur:     vctray.BackgroundBlurFull,
			portraitRelighting: false,
		},
		{
			name:               "backgroundblur_off_portraitrelighting_on",
			backgroundBlur:     vctray.BackgroundBlurOff,
			portraitRelighting: true,
		},
		{
			name:               "backgroundblur_light_portraitrelighting_on",
			backgroundBlur:     vctray.BackgroundBlurLight,
			portraitRelighting: true,
		},
		{
			name:               "backgroundblur_full_portraitrelighting_on",
			backgroundBlur:     vctray.BackgroundBlurFull,
			portraitRelighting: true,
		},
	}

	for _, subTest := range subTests {
		s.Run(ctx, subTest.name, func(ctx context.Context, s *testing.State) {
			if err := vcTray.SetCameraEffects(subTest.backgroundBlur, subTest.portraitRelighting)(ctx); err != nil {
				s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting %v: %v",
					subTest.backgroundBlur, subTest.portraitRelighting, err)
			}

			d, err := screenshot.NewDifferFromChrome(ctx, s, cr,
				screenshot.Config{
					DefaultOptions: screenshot.Options{
						WindowState: ash.WindowStateDefault,
					},
					SkipDpiNormalization: true,
				})
			if err != nil {
				s.Fatal("Failed to start screen differ: ", err)
			}
			defer d.DieOnFailedDiffs()
			if err := d.Diff(ctx, subTest.name, fakehtml.VideoNode,
				screenshot.Retries(5),
				screenshot.RetryInterval(time.Second),
			)(ctx); err != nil {
				s.Fatal("Failed the skia gold diff: ", err)
			}
		})
	}
}
