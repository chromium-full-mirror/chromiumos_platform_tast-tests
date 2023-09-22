// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIExpert,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies the expert options",
		Contacts:     []string{"chromeos-camera-eng@google.com", "dorahkim@chromium.org", "shik@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", "arc_camera3"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

// CCAUIExpert verifies expert mode related options in CCA.
func CCAUIExpert(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	for i, action := range []struct {
		Name    string
		Func    func(context.Context, *cca.App) error
		Enabled bool
	}{
		// Expert mode is not reset after each test for persistency
		{"toggleExpertMode", toggleExpertMode, false},
		{"enableExpertModeOptions", enableExpertModeOptions, true},
		{"switchModeAndBack", switchModeAndBack, true},
		{"toggleExpertMode", toggleExpertMode, false},
		{"toggleExpertMode", toggleExpertMode, true},
		{"disableExpertModeOptions", disableExpertModeOptions, false},
		{"disableExpertModeOnUI", disableExpertModeOnUI, false},
		{"enableExpertModeOnUI", enableExpertModeOnUI, false},
	} {
		if err := action.Func(ctx, app); err != nil {
			s.Fatalf("Failed to perform action %v of test %v: %v", action.Name, i, err)
		}
		if err := verifyExpertMode(ctx, app, action.Enabled); err != nil {
			s.Fatalf("Failed in test %v %v(): %v", i, action.Name, err)
		}
	}
}

func verifyExpertMode(ctx context.Context, app *cca.App, enabled bool) error {
	if err := app.WaitForVisibleState(ctx, cca.PreviewResolution, enabled); err != nil {
		return errors.Wrapf(err, "failed to wait for metadata visibility to set to %v", enabled)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		return err
	}
	return nil
}

func toggleExpertMode(ctx context.Context, app *cca.App) error {
	_, err := app.ToggleExpertMode(ctx)
	return err
}

func enableExpertModeOptions(ctx context.Context, app *cca.App) error {
	return toggleExpertModeOptions(ctx, app, true)
}

func disableExpertModeOptions(ctx context.Context, app *cca.App) error {
	return toggleExpertModeOptions(ctx, app, false)
}

func toggleExpertModeOptions(ctx context.Context, app *cca.App, enabled bool) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.MainMenu)

	if err := app.OpenSettingMenu(ctx, cca.ExpertMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.ExpertMenu)

	if err := app.SetOptionChecked(ctx, cca.ShowMetadataOption, enabled); err != nil {
		return err
	}
	if err := app.SetOptionChecked(ctx, cca.SaveMetadataOption, enabled); err != nil {
		return err
	}
	return nil
}

func switchModeAndBack(ctx context.Context, app *cca.App) error {
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		return errors.Wrap(err, "failed to switch back to photo mode")
	}
	return nil
}

func disableExpertModeOnUI(ctx context.Context, app *cca.App) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.MainMenu)

	if err := app.OpenSettingMenu(ctx, cca.ExpertMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.ExpertMenu)

	if err := app.SetOptionChecked(ctx, cca.ExpertModeOption, false); err != nil {
		return err
	}
	return nil
}

func enableExpertModeOnUI(ctx context.Context, app *cca.App) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return err
	}
	defer app.CloseSettingMenu(cleanupCtx, cca.MainMenu)

	// Clicking setting header 5 times should enable expert mode. (b/190696285)
	for i := 0; i < 5; i++ {
		if err := app.Click(ctx, cca.SettingsHeader); err != nil {
			return err
		}
	}

	if err := app.WaitForState(ctx, "expert", true); err != nil {
		return err
	}

	return nil
}
