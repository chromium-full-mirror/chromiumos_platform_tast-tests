// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"image"
	"strconv"
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
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
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

func CameraEffectsChromeResolution(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	ui := uiauto.New(tconn)

	url := srvURL + effectshtml.PageURL

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

	for _, subTest := range subTests {
		s.Run(ctx, subTest.name, func(ctx context.Context, s *testing.State) {
			urlWithResolution := url + strconv.Itoa(subTest.resolution)

			if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, urlWithResolution, vcTray); err != nil {
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

			// Only test camera effects in different resolution with BackgroundBlurFull.
			if err := vcTray.SetCameraEffects(vctray.BackgroundBlurFull, false)(ctx); err != nil {
				s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting on: %v",
					vctray.BackgroundBlurFull, err)
			}

			var imageAfter image.Image
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				// Take a screenshot after camera effects applied.
				imageAfter, err = effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
				if err != nil {
					return err
				}

				notChangedThreshold := 0.2
				changedThreshold := 0.45
				notChanged, changed := effectshtml.ImageDiff(imageBefore, imageAfter, 0.0)
				if notChanged < notChangedThreshold || changed < changedThreshold {
					return errors.Errorf("Wrong percentage of pixel change: %f changed and %f not changed", changed, notChanged)
				}

				return nil

			}, &testing.PollOptions{Timeout: 3 * time.Second, Interval: time.Second}); err != nil {
				effectshtml.SaveImageToFaillog(ctx, s, imageBefore, effectshtml.BeforeEffectsImageName)
				effectshtml.SaveImageToFaillog(ctx, s, imageAfter, subTest.name+effectshtml.AfterEffectsImageName)
				s.Fatal("Screenshot diff unexpected: ", err)
			}

			if err := effectshtml.CloseURLAndWaitForWindowGone(ctx, tconn, br, urlWithResolution, vcTray); err != nil {
				s.Fatal("Fail to wait for close tab: ", err)
			}
		})
	}
}
