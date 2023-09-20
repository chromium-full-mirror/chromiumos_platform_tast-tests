// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUILayout,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test to verify the layout of Chrome Camera App",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"informational", "group:mainline", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Data:         []string{"blank_1280x720.mjpeg"},
		Timeout:      3 * time.Minute,
		HardwareDeps: cca.DeviceWithLayoutMonitored,
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
		Vars:         screenshot.ScreenDiffVars,
	})
}

const (
	diffWindowWidth  = 800
	diffWindowHeight = 600
)

// CCAUILayout takes screenshots and compares to the ground truth on the Gold server to ensure no unexpected UI changes are introduced.
func CCAUILayout(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(cca.FixtureData).Chrome
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene

	if err := switchScene(ctx, cca.SceneData{Path: s.DataPath("blank_1280x720.mjpeg")}); err != nil {
		s.Fatal("Failed to set up fake scene: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get if the DUT's tablet mode is enabled: ", err)
	}

	defaultWindowState := ash.WindowStateNormal
	if tabletMode {
		// WindowStateNormal is invalid in the tablet mode.
		defaultWindowState = ash.WindowStateDefault
	}

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
		s.Fatal("Failed to start a screen differ: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, differ.Tconn())
	defer differ.DieOnFailedDiffs()

	if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
		// TODO(b/285276951): Replace with a better way to workaround such issue.
		// GoBigSleepLint - Sleep to wait for the multitask menu indicator to dismiss.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep to wait for the multitask menu indicator dismiss")
		}

		return checkAppLayout(ctx, app, differ)
	}, cca.TestWithAppParams{}); err != nil {
		s.Fatal("Failed when checking app layout: ", err)
	}
}

func checkAppLayout(ctx context.Context, app *cca.App, differ screenshot.Differ) error {
	if err := app.HideFloatingUI(ctx); err != nil {
		return errors.Wrap(err, "failed to hide floating UI")
	}
	// Verify photo/video/scan mode.
	if err := checkEachModes(ctx, app, differ); err != nil {
		return errors.Wrap(err, "failed to check the layout for each modes")
	}

	// Verify Settings page and expert mode.
	if err := checkSettingsPage(ctx, app, differ); err != nil {
		return errors.Wrap(err, "failed to check the layout for the settings page")
	}

	// Verify options panels.
	if err := checkPreviewOptions(ctx, app, differ); err != nil {
		return errors.Wrap(err, "failed to check the layout for preview options")
	}
	return nil
}

func checkEachModes(ctx context.Context, app *cca.App, differ screenshot.Differ) error {
	for _, tc := range []struct {
		mode     cca.Mode
		pageName string
	}{{
		cca.Photo,
		"photoMode",
	}, {
		cca.Video,
		"videoMode",
	}, {
		cca.Scan,
		"scanMode",
	}} {
		if err := app.SwitchMode(ctx, tc.mode); err != nil {
			return errors.Wrapf(err, "failed to switch to %v mode", tc.mode)
		}
		if err := diffPage(ctx, app, differ, tc.pageName); err != nil {
			return errors.Wrapf(err, "failed to diff window for %v page", tc.pageName)
		}
	}
	return nil
}

func checkSettingsPage(ctx context.Context, app *cca.App, differ screenshot.Differ) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Switch back to photo mode.
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		return errors.Wrap(err, "failed to switch to photo mode")
	}

	// Enable expert mode first.
	if err := app.EnableExpertMode(ctx); err != nil {
		return errors.Wrap(err, "failed to enable expert mode")
	}

	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return errors.Wrap(err, "failed to click settings button")
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.MainMenu)

	if err := diffPage(ctx, app, differ, "settings"); err != nil {
		return errors.Wrap(err, "failed to diff window for settings page")
	}

	// Verify expert mode content.
	if err := app.OpenSettingMenu(ctx, cca.ExpertMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.ExpertMenu)

	if err := diffPage(ctx, app, differ, "expertMode"); err != nil {
		return errors.Wrap(err, "failed to diff window for expert mode page")
	}
	return nil
}

func checkPreviewOptions(ctx context.Context, app *cca.App, differ screenshot.Differ) error {
	// Switch back to photo mode.
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		return errors.Wrap(err, "failed to switch to photo mode")
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a keyboard")
	}
	defer keyboard.Close(ctx)

	// Verify panels.
	for _, panel := range []struct {
		name   string
		button cca.UIComponentName
	}{{
		"mirrorPanel",
		cca.OpenMirrorPanelButton,
	}, {
		"gridPanel",
		cca.OpenGridPanelButton,
	}, {
		"timerPanel",
		cca.OpenTimerPanelButton,
	}} {
		if err := app.Click(ctx, panel.button); err != nil {
			return errors.Wrapf(err, "failed to click %v button", panel.name)
		}
		if err := diffPage(ctx, app, differ, panel.name); err != nil {
			return errors.Wrapf(err, "failed to diff window for %v", panel.name)
		}
		if err := keyboard.Accel(ctx, "esc"); err != nil {
			return errors.Wrap(err, "failed to dismiss the option panel")
		}
	}
	return nil
}

func diffPage(ctx context.Context, app *cca.App, differ screenshot.Differ, pageName string) error {
	// Diff the window.
	return differ.DiffWindow(ctx, pageName)(ctx)
}
