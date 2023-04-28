// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"chromiumos/tast/local/camera/cca"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/screenshot"
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
		Attr:         []string{"informational", "group:mainline", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Data:         []string{"blank_1280x720.mjpeg"},
		HardwareDeps: cca.DeviceWithLayoutMonitored,
		BugComponent: "b:978428",
		Fixture:      "ccaTestBridgeReadyWithFakeCamera",
		Vars:         screenshot.ScreenDiffVars,
	})
}

const (
	diffWindowWidth  = 800
	diffWindowHeight = 600
)

func CCAUILayout(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(cca.FixtureData).Chrome
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene

	if err := switchScene(s.DataPath("blank_1280x720.mjpeg")); err != nil {
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
			WindowWidthDP:  diffWindowWidth,
			WindowHeightDP: diffWindowHeight,
			WindowState:    defaultWindowState,
			RetryInterval:  2 * time.Second,
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
		return checkAppLayout(ctx, app, differ)
	}, cca.TestWithAppParams{}); err != nil {
		s.Fatal("Failed when checking app layout: ", err)
	}
}

func checkAppLayout(ctx context.Context, app *cca.App, differ screenshot.Differ) error {
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

	if err := cca.MainMenu.Open(ctx, app); err != nil {
		return errors.Wrap(err, "failed to click settings button")
	}
	defer cca.MainMenu.Close(cleanupCtx, app)

	if err := diffPage(ctx, app, differ, "settings"); err != nil {
		return errors.Wrap(err, "failed to diff window for settings page")
	}

	// Verify expert mode content.
	if err := cca.ExpertMenu.Open(ctx, app); err != nil {
		return err
	}
	defer cca.ExpertMenu.Close(ctx, app)

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
		button cca.UIComponent
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
	return differ.DiffWindow(ctx, pageName, screenshot.Retries(2))(ctx)
}
