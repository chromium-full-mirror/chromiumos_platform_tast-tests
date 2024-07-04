// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"image"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effectshtml"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraEffectsChrome,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify camera effects using screen test",
		Contacts: []string{
			"cros-video-conference-tast-tests@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Attr: []string{
			"group:camera_dependent",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
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
				Name:    "ash",
				Fixture: fixture.LoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:    "lacros",
				Fixture: fixture.LoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
		},
	})
}

func CameraEffectsChrome(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	ui := uiauto.New(tconn)

	url := srvURL + effectshtml.PageURL
	if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}

	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurOff, false)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting off: %v",
			vctray.BackgroundBlurOff, err)
	}

	// Take a screenshot before camera effects applied.
	imageBefore, err := effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Fail to grab camera screen shot before: ", err)
	}

	// Run subtests to verify video effects are correctly applied.
	// Note: Golden images can be found at https://cros-tast-gold.skia.org/list?corpus=videoconferencing.
	subTests := []struct {
		name                string
		backgroundBlur      vctray.BackgroundBlurLevel
		portraitRelighting  bool
		notChangedThreshold float64
		changedThreshold    float64
	}{
		{
			name:                "backgroundblur_off_portraitrelighting_off",
			backgroundBlur:      vctray.BackgroundBlurOff,
			portraitRelighting:  false,
			notChangedThreshold: 0.95,
			changedThreshold:    0.0,
		},
		{
			name:                "backgroundblur_light_portraitrelighting_off",
			backgroundBlur:      vctray.BackgroundBlurLight,
			portraitRelighting:  false,
			notChangedThreshold: 0.2,
			changedThreshold:    0.4,
		},
		{
			name:                "backgroundblur_full_portraitrelighting_off",
			backgroundBlur:      vctray.BackgroundBlurFull,
			portraitRelighting:  false,
			notChangedThreshold: 0.2,
			changedThreshold:    0.4,
		},
		{
			name:                "backgroundblur_off_portraitrelighting_on",
			backgroundBlur:      vctray.BackgroundBlurOff,
			portraitRelighting:  true,
			notChangedThreshold: 0.6,
			changedThreshold:    0.15,
		},
		{
			name:                "backgroundblur_light_portraitrelighting_on",
			backgroundBlur:      vctray.BackgroundBlurLight,
			portraitRelighting:  true,
			notChangedThreshold: 0.10,
			changedThreshold:    0.60,
		},
		{
			name:                "backgroundblur_full_portraitrelighting_on",
			backgroundBlur:      vctray.BackgroundBlurFull,
			portraitRelighting:  true,
			notChangedThreshold: 0.10,
			changedThreshold:    0.60,
		},
	}

	for _, subTest := range subTests {
		s.Run(ctx, subTest.name, func(ctx context.Context, s *testing.State) {
			if err := vcTray.SetCameraEffects(subTest.backgroundBlur, subTest.portraitRelighting)(ctx); err != nil {
				s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting %v: %v",
					subTest.backgroundBlur, subTest.portraitRelighting, err)
			}

			var imageAfter image.Image
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				// Take a screenshot after camera effects applied.
				imageAfter, err = effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
				if err != nil {
					return err
				}

				notChanged, changed := effectshtml.ImageDiff(imageBefore, imageAfter, 0.0)
				if notChanged < subTest.notChangedThreshold || changed < subTest.changedThreshold {
					return errors.Errorf("Wrong percentage of pixel change for %s: %f changed and %f not changed", subTest.name, changed, notChanged)
				}

				return nil

			}, &testing.PollOptions{Timeout: 3 * time.Second, Interval: time.Second}); err != nil {
				effectshtml.SaveImageToFaillog(ctx, s, imageBefore, effectshtml.BeforeEffectsImageName)
				effectshtml.SaveImageToFaillog(ctx, s, imageAfter, subTest.name+effectshtml.AfterEffectsImageName)
				s.Fatal("Screenshot diff unexpected: ", err)
			}
		})
	}
}
