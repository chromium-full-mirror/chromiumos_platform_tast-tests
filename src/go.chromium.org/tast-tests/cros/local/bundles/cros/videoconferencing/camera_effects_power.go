// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effectshtml"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
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
			"group:crosbolt", "crosbolt_perbuild",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
		Timeout:      15 * time.Minute,
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
			data.BackgroundImageJpg,
			data.BackgroundMetadata,
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

func CameraEffectsPower(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	url := srvURL + effectshtml.PageURL
	if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}

	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurOff, false)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting off: %v",
			vctray.BackgroundBlurOff, err)
	}

	param, ok := s.Param().(effectsParams)
	if !ok {
		s.Fatal("Failed to convert test effectsParams")
	}

	// Copy camera background to the backgroundImageDir to apply.
	if param.blurLevel == vctray.BackgroundBlurImage {
		userPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to get user's userPath path: ", err)
		}

		imagePath := filepath.Join(userPath, data.BackgroundImageDirname)

		if err := os.MkdirAll(imagePath, 0777); err != nil {
			s.Fatal("Failed to create image path: ", err)
		}
		if err := fsutil.CopyFile(
			s.DataPath(data.BackgroundImageJpg), filepath.Join(imagePath, data.BackgroundImageJpg)); err != nil {
			s.Fatal("Failed to copy image to custom-camera-backgrounds: ", err)
		}
		if err := fsutil.CopyFile(
			s.DataPath(data.BackgroundMetadata), filepath.Join(imagePath, data.BackgroundMetadata)); err != nil {
			s.Fatal("Failed to copy metadata to custom-camera-backgrounds: ", err)
		}
	}

	// Set camera effects.
	if err := vcTray.SetCameraEffects(param.blurLevel, param.relightEnabled)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting %v: %v",
			param.blurLevel, param.relightEnabled, err)
	}

	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
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
