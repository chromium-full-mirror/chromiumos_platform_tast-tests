// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakehtml"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
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
		Func:         CameraEffectsChromeResolution,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks Camera Effects in different resolution",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
		},
		Attr: []string{
			"group:camera_dependent",
			"group:video_conference",
			"video_conference_per_build",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Vars:         screenshot.ScreenDiffVars,
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
		SearchFlags: []*testing.StringPair{
			{
				// Resolution change for camera.
				Key:   "feature_id",
				Value: "screenplay-0a0d786d-a9ec-4d9b-a9ed-0578f464de43",
			},
		},
	})
}

func CameraEffectsChromeResolution(ctx context.Context, s *testing.State) {
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

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, url)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	fakeHTMLUI := fakehtml.NewUI(tconn)
	if err := fakeHTMLUI.MayBeAllowCameraAccess(ctx); err != nil {
		s.Fatal("Failed to allow camera access: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	vcTray := vctray.New(ctx, tconn)
	// Only test camera effects in different resolution with BackgroundBlurFull and RelightingOn.
	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurFull, true)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting on: %v",
			vctray.BackgroundBlurFull, err)
	}

	// Run subtests to verify camera effects are correctly applied in different resolution.
	// Note: Golden images can be found at https://cros-tast-gold.skia.org/list?corpus=videoconferencing.
	subTests := []struct {
		name       string
		resolution int
	}{
		// Note: 720p has already been covered in CameraEffectsChrome test as the default value.
		{
			name:       "1080p",
			resolution: 1080,
		},
		{
			name:       "360p",
			resolution: 360,
		},
		{
			name:       "180p",
			resolution: 180,
		},
	}

	ui := uiauto.New(tconn)

	for _, subTest := range subTests {
		s.Run(ctx, subTest.name, func(ctx context.Context, s *testing.State) {
			urlWithResolution := url + strconv.Itoa(subTest.resolution)
			if err := conn.Navigate(ctx, urlWithResolution); err != nil {
				s.Fatalf("Failed to navigate to %q: %v", urlWithResolution, err)
			}

			if err := ui.WithTimeout(time.Minute).WaitUntilExists(fakehtml.VideoNode)(ctx); err != nil {
				s.Fatal("Failed to fully load the page : ", err)
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
