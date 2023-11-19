// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPortraitMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that CCA can take portrait mode photo",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent"},
		SoftwareDeps: []string{"camera_app", "camera_feature_portrait_mode", "chrome", caps.BuiltinOrVividCamera},
		Data:         []string{"pink-nature-1920x1080.jpg", "portrait_4096x3072.jpg", "portrait_gt_20231120.jpg"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
		Vars:         screenshot.ScreenDiffVars,
	})
}

// CCAUIPortraitMode tests that portrait mode works expectedly.
func CCAUIPortraitMode(ctx context.Context, s *testing.State) {
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true})

	for _, tc := range []struct {
		scenePath                 string
		hasHumanFace              bool
		expectedPortraitOutputImg string
	}{
		{"pink-nature-1920x1080.jpg", false, ""},
		{"portrait_4096x3072.jpg", true, "portrait_gt_20231120.jpg"},
	} {
		if err := switchScene(ctx, cca.SceneData{Path: s.DataPath(tc.scenePath)}); err != nil {
			s.Fatal("Failed to prepare portrait scene: ", err)
		}

		// TODO(b/309572841): Remove the temporary Sleep() call after verifying that the scene is updated after switchScene() is called.
		// GoBigSleepLint: Wait for 2 second for the test scene to be switched.
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		app := s.FixtValue().(cca.FixtureData).App()

		if err := app.SwitchMode(ctx, cca.Portrait); err != nil {
			s.Fatal("Failed to switch to portrait mode: ", err)
		}

		outputFiles, err := app.TakePortraitPhoto(ctx, cca.TimerOff, tc.hasHumanFace)
		if err != nil {
			s.Fatal("Failed to take portrait photo: ", err)
		}

		expectedNumOutput := 1
		if tc.hasHumanFace {
			expectedNumOutput = 2
		}
		if expectedNumOutput != len(outputFiles) {
			s.Fatalf("Expected %d output files, but got %d", expectedNumOutput, len(outputFiles))
		}

		if expectedNumOutput == 2 {
			if err := comparePortraitToGroundTruth(ctx, s, app); err != nil {
				s.Fatal("Failed to compare portrait photo to ground truth: ", err)
			}
		}
	}
}

func comparePortraitToGroundTruth(ctx context.Context, s *testing.State, app *cca.App) error {
	if err := app.Click(ctx, cca.GalleryButton); err != nil {
		return errors.Wrap(err, "failed to click the gallery button")
	}

	cr := s.FixtValue().(cca.FixtureData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}

	tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get if the DUT's tablet mode is enabled")
	}

	defaultWindowState := ash.WindowStateNormal
	if tabletMode {
		// WindowStateNormal is invalid in the tablet mode.
		defaultWindowState = ash.WindowStateDefault
	}

	const (
		diffWindowWidth  = 800
		diffWindowHeight = 600
	)

	screendiffConfig := screenshot.Config{
		DefaultOptions: screenshot.Options{
			WindowWidthDP:       diffWindowWidth,
			WindowHeightDP:      diffWindowHeight,
			WindowState:         defaultWindowState,
			Retries:             8,
			RetryInterval:       500 * time.Millisecond,
			MaxDifferentPixels:  30,
			PixelDeltaThreshold: 9,
		},
		SkipDpiNormalization: true,
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	differ, err := screenshot.NewDifferFromChrome(ctx, s, cr, screendiffConfig)
	if err != nil {
		return errors.Wrap(err, "failed to start a screen differ")
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, differ.Tconn())
	defer differ.DieOnFailedDiffs()

	if err = differ.DiffWindow(ctx, "portrait")(ctx); err != nil {
		return errors.Wrap(err, "failed to diff the portrait image")
	}

	return nil
}
