// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakehtml"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	backgroundImageDir  = "custom-camera-backgrounds/original" // relative path in user path to store background images.
	backgroundImageFile = "camera_background.jpg"

	testDuration = 5 * time.Minute
)

type effectsParams struct {
	// Blur level, vctray.BackgroundBlurOff means off, and vctray.BackgroundBlurImage means background replace.
	blurLevel vctray.BackgroundBlurLevel

	// Whether to enable portrait relight or not.
	relightEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraEffectsPower,
		LacrosStatus: testing.LacrosVariantUnneeded, // Browser only used to trigger VC UI.
		Desc:         "Checks camera effects power usage",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"charleszhao@google.com",
		},
		BugComponent: "b:187682",
		Attr: []string{
			"group:camera_dependent",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(false)},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Timeout:      15 * time.Minute,
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
			backgroundImageFile,
		},
		Fixture: fixture.PowerLoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder,
		Params: []testing.Param{
			{
				Name: "no_effects",
				Val: effectsParams{
					blurLevel:      vctray.BackgroundBlurOff,
					relightEnabled: false,
				},
			},
			{
				Name: "blur_only",
				Val: effectsParams{
					blurLevel:      vctray.BackgroundBlurFull,
					relightEnabled: false,
				},
			},
			{
				Name: "relight_only",
				Val: effectsParams{
					blurLevel:      vctray.BackgroundBlurOff,
					relightEnabled: true,
				},
			},
			{
				Name: "replace_only",
				Val: effectsParams{
					blurLevel:      vctray.BackgroundBlurImage,
					relightEnabled: false,
				},
			},
		},
	})
}

func CameraEffectsPower(ctx context.Context, s *testing.State) {
	// Shorten context to allow for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	param, ok := s.Param().(effectsParams)
	if !ok {
		s.Fatal("Failed to convert test effectsParams")
	}

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Open video on simple javascript browser.
	testing.ContextLog(ctx, "Opening Simple Meeting")
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer srv.Close()

	url := srv.URL + fakehtml.PageURL
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, url)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	fakeHTMLUI := fakehtml.NewUI(tconn)
	if err := fakeHTMLUI.MayBeAllowCameraAccess(ctx); err != nil {
		s.Fatal("Failed to allow camera access: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	// Copy camera_background.jpg to the backgroundImageDir to apply.
	if param.blurLevel == vctray.BackgroundBlurImage {
		userPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to get user's userPath path: ", err)
		}

		imagePath := filepath.Join(userPath, backgroundImageDir)

		if err := os.MkdirAll(imagePath, 0777); err != nil {
			s.Fatal("Failed to create image path: ", err)
		}
		if err := fsutil.CopyFile(
			s.DataPath(backgroundImageFile), filepath.Join(imagePath, backgroundImageFile)); err != nil {
			s.Fatal("Failed to copy file to custom-camera-backgrounds: ", err)
		}
	}

	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Set camera effects.
	if err := vcTray.SetCameraEffects(param.blurLevel, param.relightEnabled)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting %v: %v",
			param.blurLevel, param.relightEnabled, err)
	}

	// Start to track power metrics.
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: Keep camera effects for the testDuration to measure the power usage.
	if err := testing.Sleep(ctx, testDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
