// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effectshtml"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/effects"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	testDuration   = 5 * time.Minute
	warmupDuration = 5 * time.Second
)

type effectsParams struct {
	// Blur level, effects.BlurDisabled means off, and effects.BlurImage means background replace.
	blurLevel effects.BlurLevel

	// Whether to enable portrait relight or not.
	relightEnabled bool

	// Whether to enable face retouch or not.
	retouchEnabled bool
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
					blurLevel:      effects.KBlurDisabled,
					relightEnabled: false,
					retouchEnabled: false,
				},
			},
			{
				Name: "blur_only",
				Val: effectsParams{
					blurLevel:      effects.KBlurMaximum,
					relightEnabled: false,
					retouchEnabled: false,
				},
			},
			{
				Name: "relight_only",
				Val: effectsParams{
					blurLevel:      effects.KBlurDisabled,
					relightEnabled: true,
					retouchEnabled: false,
				},
			},
			{
				Name: "replace_only",
				Val: effectsParams{
					blurLevel:      effects.KBlurImage,
					relightEnabled: false,
					retouchEnabled: false,
				},
			},
			{
				Name: "retouch_only",
				Val: effectsParams{
					blurLevel:      effects.KBlurDisabled,
					relightEnabled: false,
					retouchEnabled: true,
				},
			},
			{
				Name: "relight_and_retouch",
				Val: effectsParams{
					blurLevel:      effects.KBlurDisabled,
					relightEnabled: true,
					retouchEnabled: true,
				},
			},
		},
	})
}

func CameraEffectsPower(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, _, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	url := srvURL + effectshtml.PageURL
	if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}

	if _, err := effects.ApplyPlatformEffects(ctx, false, false, effects.KBlurDisabled, effects.KAuto); err != nil {
		s.Fatalf("Failed to set camera effects to PortraitRelighting off; Retouch off; BackgroundBlur %v: %v",
			effects.KBlurDisabled, err)
	}

	param, ok := s.Param().(effectsParams)
	if !ok {
		s.Fatal("Failed to convert test effectsParams")
	}

	// Set camera effects.
	resetEffects, err := effects.ApplyPlatformEffects(ctx, param.relightEnabled, param.retouchEnabled, param.blurLevel, effects.KAuto)
	if err != nil {
		s.Fatalf("Failed to set camera effects to PortraitRelighting %v; Retouch %v; BackgroundBlur %v: %v",
			param.relightEnabled, param.retouchEnabled, param.blurLevel, err)
	}
	defer func() {
		if err := resetEffects(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to reset platform effects: ", err)
		}
	}()

	// GoBigSleepLint: Wait a few seconds for setting camera effects to take effect.
	if err := testing.Sleep(ctx, warmupDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
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
