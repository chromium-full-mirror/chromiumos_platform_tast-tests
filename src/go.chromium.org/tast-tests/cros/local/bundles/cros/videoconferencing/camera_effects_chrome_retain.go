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
		Func:         CameraEffectsChromeRetain,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects retains after re-launching vc apps",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:video_conference", "video_conference_per_build",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		SearchFlags: []*testing.StringPair{
			{
				// Blur retain.
				Key:   "feature_id",
				Value: "screenplay-ad05d869-0d1a-4340-832c-b08fece20bb3",
			},
			{
				// Retain Blur and lighting improvement.
				Key:   "feature_id",
				Value: "screenplay-f6e8fc90-04dc-49eb-9051-00391c045fce",
			},
			{
				// Retain relighting.
				Key:   "feature_id",
				Value: "screenplay-bff2442b-de76-422e-a45c-f077f0f6eca3",
			},
		},
		Params: []testing.Param{
			{
				Fixture: fixture.LoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
		},
		Vars: screenshot.ScreenDiffVars,
	})
}

func CameraEffectsChromeRetain(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	//  Open video on simple javascript browser.
	testing.ContextLog(ctx, "Opening Simple Meeting")
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()

	url := srv.URL + fakehtml.PageURL

	func() {
		conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, url)
		if err != nil {
			s.Fatal("Failed to launch browser: ", err)
		}
		defer closeBrowser(cleanupCtx)
		defer conn.Close()

		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

		fakeHTMLUI := fakehtml.NewUI(tconn)
		if err := fakeHTMLUI.MayBeAllowCameraAccess(ctx); err != nil {
			s.Fatal("Failed to allow camera access: ", err)
		}

		if err := uiauto.New(tconn).WaitUntilExists(fakehtml.VideoNode)(ctx); err != nil {
			s.Fatal("Camera is not working appropriately: ", err)
		}

		vcTray := vctray.New(ctx, tconn)

		if err := vcTray.SetCameraEffects(vctray.BackgroundBlurFull, true)(ctx); err != nil {
			s.Fatal("Failed to set camera effects: ", err)
		}
	}()

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, url)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

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
	if err := d.Diff(ctx, "backgroundblur_full_portraitrelighting_on", fakehtml.VideoNode,
		screenshot.Retries(5),
		screenshot.RetryInterval(time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed the skia gold diff: ", err)
	}
}
