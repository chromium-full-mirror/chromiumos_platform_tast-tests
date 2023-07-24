// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceKeyboardSixPackKeys,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Modifying a six pack key shortcut works",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Timeout:      2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

// DeviceKeyboardSixPackKeys opens the remap keys subpage, changes the shortcut
// used to trigger the "Delete" six pack key action and later uses the password
// field on the login screen to verify that the "Delete" key action is triggered
// by the selected shortcut.
func DeviceKeyboardSixPackKeys(ctx context.Context, s *testing.State) {
	var creds chrome.Creds
	cr, err := remapSixPackKey(ctx, s)
	if err != nil {
		s.Fatal("Remapping the Delete six pack key failed: ", err)
	}

	creds = cr.Creds()
	cr, err = startChrommeOnLoginScreen(ctx, s.RequiredVar("ui.signinProfileTestExtensionManifestKey"))
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
	if err := uiauto.Combine("focus password field",
		ui.WaitUntilExists(field),
		ui.LeftClick(field),
		ui.WaitUntilExists(field.Focused()),
	)(ctx); err != nil {
		s.Fatal("Failed to focus password field: ", err)
	}

	kb, err := input.Keyboard(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}

	defer kb.Close(cleanupCtx)

	if err := kb.Type(ctx, creds.Pass); err != nil {
		s.Fatal("Failed to type password: ", err)
	}

	// Pressing "Left" followed by "Search+Backspace" should trigger the "Delete"
	// six pack key action and remove the last character of the entered psasword.
	if err := kb.Accel(ctx, "Left"); err != nil {
		s.Fatal("Failed to press Left: ", err)
	}

	if err := kb.Accel(ctx, "Search+Backspace"); err != nil {
		s.Fatal("Failed to press Search+Backspace: ", err)
	}

	passwordField, err := readUserPassword(ctx, tconn, creds.User)
	if err != nil {
		s.Fatal("Failed to read Password: ", err)
	}
	expectedPassword := creds.Pass[:len(creds.Pass)-1]
	if passwordField != expectedPassword {
		s.Fatalf("Passwords do not match Password Field: %q User entered value: %q", passwordField, expectedPassword)
	}
}

func startChrommeOnLoginScreen(ctx context.Context, key string) (*chrome.Chrome, error) {
	// NoLogin is used to land in signin screen.
	cr, err := chrome.New(
		ctx,
		chrome.EnableFeatures("InputDeviceSettingsSplit", "AltClickAndSixPackCustomization"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(key),
	)

	return cr, err
}

// remapSixPackKey logs in and change a six pack key shortcut in the remap keys subpage.
func remapSixPackKey(ctx context.Context, s *testing.State) (*chrome.Chrome, error) {
	cr, err := chrome.New(ctx, chrome.EnableFeatures("InputDeviceSettingsSplit", "AltClickAndSixPackCustomization"), chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))
	if err != nil {
		return cr, errors.Wrap(err, "Chrome login failed")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	// This is needed for reven tests, as login flow there relies on the existence of a device setting.
	if err := userutil.WaitForOwnership(ctx, cr); err != nil {
		return cr, errors.Wrap(err, "User did not become device owner")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return cr, errors.Wrap(err, "failed to connect to Test API")
	}

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		return cr, errors.Wrap(err, "failed to open setting page")
	}
	defer settings.Close(cleanupCtx)

	if err := uiauto.Combine("open customize keyboard keys page",
		ui.DoDefault(constants.KeyboardRow),
		ui.DoDefault(constants.CustomizeKeyboardKeys),
	)(ctx); err != nil {
		return cr, errors.Wrap(err, "failed to open customize keyboard keys page")
	}

	if err := devicesettings.Remap(ctx, ui, constants.DeleteSixPackKey, constants.DeleteAltShortcut); err != nil {
		return cr, errors.Wrap(err, "failed to remap")
	}

	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return cr, errors.Wrap(err, "failed to restart ui")
	}
	return cr, nil
}

func readUserPassword(ctx context.Context, tconn *chrome.TestConn, username string) (string, error) {
	if err := lockscreen.ShowPassword(ctx, tconn); err != nil {
		return "", errors.Wrap(err, "failed to click the show password button")
	}

	password, err := lockscreen.UserPassword(ctx, tconn, username, false)
	if err != nil {
		return "", errors.Wrap(err, "failed to read Password")
	}
	return password.Value, nil
}
