// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
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
		Func:         CCAEffects,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Checks Video Effects in built-in Camera App",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:video_conference",
			"video_conference_per_build",
		},
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		// Each parameterized test contains multiple subtests.
		// Using -var "subtests" to make it possible limiting the subtests to run.
		// e.g. tast run -var=subtests=backgroundblur_off_portraitrelighting_off
		// <dut> videoconferencing.CCAEffects.clamshell
		// It should only be used for local debugging, not used to filter tests on mainline.
		Vars: append(screenshot.ScreenDiffVars, "subtests"),
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
	})
}

func CCAEffects(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.Camera.ID); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer apps.Close(cleanupCtx, tconn, apps.Camera.ID)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	if err := uiauto.New(tconn).WaitUntilExists(cca.A11yCanvasNode)(ctx); err != nil {
		s.Fatal("Camera is not working appropriately: ", err)
	}

	// Maximize VC app window in clamshell mode to reduce resolution noises on different devices.
	// Skip Tablet mode as app is full screen by default.
	if inTabletMode, err := ash.TabletModeEnabled(ctx, tconn); err != nil {
		s.Fatal("Failed to get tablet-mode status: ", err)
	} else if !inTabletMode {
		if _, err := ash.MaximizeWindowTitleContains(ctx, tconn, "Camera"); err != nil {
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

	enabledSubtests := make(map[string]struct{})
	subtestsVar, ok := s.Var("subtests")
	if ok {
		testing.ContextLog(ctx, "Enabled subtests: ", subtestsVar)
		for _, subTest := range strings.Split(subtestsVar, ",") {
			enabledSubtests[subTest] = struct{}{}
		}
	}

	for _, subTest := range subTests {
		// Check whether this subtest is enabled in the test var.
		if len(enabledSubtests) > 0 {
			if _, ok := enabledSubtests[subTest.name]; !ok {
				continue
			}
		}

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
			if err := d.Diff(ctx, subTest.name, cca.A11yCanvasNode,
				screenshot.Retries(5),
				screenshot.RetryInterval(time.Second),
			)(ctx); err != nil {
				s.Fatal("Failed the skia gold diff: ", err)
			}
		})
	}
}
