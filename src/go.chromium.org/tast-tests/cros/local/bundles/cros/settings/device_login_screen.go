// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/devicesettings"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceLoginScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Device settings work on the login screen",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			ui.GaiaPoolDefaultVarName,
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Timeout:      2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

// DeviceLoginScreen opens the remap keys subpage, remaps Ctrl with Backspace
// and later uses the password field on the login screen to verify that the
// modifier key remapping setting change works on the login screen.
func DeviceLoginScreen(ctx context.Context, s *testing.State) {
	var creds chrome.Creds
	// Log in and remap a modifier key in the remap keys subpage.
	// Logging in and out will also create a user pod on the login screen that
	// we can use to verify keyboard settings.
	func() {
		cr, err := chrome.New(ctx, chrome.EnableFeatures("InputDeviceSettingsSplit"), chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))
		if err != nil {
			s.Fatal("Chrome login failed: ", err)
		}

		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
			"ui_dump")
		creds = cr.Creds()

		// This is needed for reven tests, as login flow there relies on the existence of a device setting.
		if err := userutil.WaitForOwnership(ctx, cr); err != nil {
			s.Fatal("User did not become device owner: ", err)
		}
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to Test API: ", err)
		}

		ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

		s.Log("Open setting page and starting test")
		settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
		if err != nil {
			s.Fatal("Failed to open setting page: ", err)
		}
		defer settings.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

		// Find Keyboard row and click it.
		if err := ui.DoDefault(constants.KeyboardRow)(ctx); err != nil {
			s.Fatal("Failed to click keyboard row: ", err)
		}

		// Click customize keyboard keys row and verify if all the buttons show up.
		if err := ui.DoDefault(constants.CustomizeKeyboardKeys)(ctx); err != nil {
			s.Fatal("Failed to click Customize keyboard keys row: ", err)
		}

		err = devicesettings.Remap(ctx, ui, constants.Control, constants.Backspace)
		if err != nil {
			s.Fatal("Failed to remap: ", err)
		}

		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	// NoLogin is used to land in signin screen.
	cr, err := chrome.New(
		ctx,
		chrome.EnableFeatures("InputDeviceSettingsSplit"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)

	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	if err = lockscreen.WaitForPasswordField(ctx, tconn, creds.User, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for password field: ", err)
	}

	field, err := lockscreen.PasswordFieldFinder(creds.User)
	if err != nil {
		s.Fatal("Failed to find password field: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(field)(ctx); err != nil {
		s.Fatal("Failed to find password box: ", err)
	}

	if err := ui.LeftClick(field)(ctx); err != nil {
		s.Fatal("Failed to click password box: ", err)
	}

	// Wait for the field to be focused before entering the password.
	if err := ui.WaitUntilExists(field.Focused())(ctx); err != nil {
		s.Fatal("Password field not focused yet: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}

	defer kb.Close(ctx)
	if err := kb.Type(ctx, creds.Pass); err != nil {
		s.Fatal("Failed to type password: ", err)
	}

	if err := lockscreen.ShowPassword(ctx, tconn); err != nil {
		s.Fatal("Failed to click the Show password button: ", err)
	}

	passwordField, err := lockscreen.UserPassword(ctx, tconn, creds.User, false)
	if err != nil {
		s.Fatal("Failed to read Password: ", err)
	}

	if passwordField.Value != creds.Pass {
		s.Fatalf("Passwords do not match Password Field: %q User entered value: %q", passwordField.Value, creds.Pass)
	}

	if err := kb.Accel(ctx, "Ctrl"); err != nil {
		s.Fatal("Failed to press Ctrl: ", err)
	}

	passwordField, err = lockscreen.UserPassword(ctx, tconn, creds.User, false)
	if err != nil {
		s.Fatal("Failed to read Password: ", err)
	}

	if passwordField.Value == creds.Pass {
		s.Fatal("Passwords unexpectedly match. Remapping Ctrl -> Backspace did not persist to login screen settings")
	}
}
