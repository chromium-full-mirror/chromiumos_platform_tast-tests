// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/bundles/cros/videoconferencing/fixture"
	"chromiumos/tast/local/camera/cca"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
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
			"group:mainline", "informational",
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

	ui := uiauto.New(tconn)
	// The toast message is above the canvas, affecting the screenshot test.
	if err := uiauto.IfSuccessThen(
		ui.WithTimeout(3*time.Second).WaitUntilExists(cca.ToastNode),
		ui.WaitUntilGone(cca.ToastNode),
	)(ctx); err != nil {
		s.Fatal("Failed to handle toast message: ", err)
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
		// TODO(b/267709319): Add screen tests with portrait relighting on.
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
