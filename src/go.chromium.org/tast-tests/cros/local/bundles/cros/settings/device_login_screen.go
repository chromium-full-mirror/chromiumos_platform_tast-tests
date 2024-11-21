// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/devicesettings"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceLoginScreen,
		Desc: "Device keyboard remapping settings work on the login screen",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:release-health",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			ui.GaiaPoolDefaultVarName,
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Timeout:      2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

// DeviceLoginScreen opens the remap keys subpage, remaps Ctrl with Backspace
// and later uses the password field on the login screen to verify that the
// modifier key remapping setting change works on the login screen.
func DeviceLoginScreen(ctx context.Context, s *testing.State) {
	username := "test123@gmail.com"
	password := "pass"
	// Log in and remap a modifier key in the remap keys subpage.
	// Logging in and out will also create a user pod on the login screen that
	// we can use to verify keyboard settings.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()
	cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: username, Pass: password}))
	defer userutil.ResetUsers(cleanupCtx)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)
	s.Log("Opening device settings page")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open device settings page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Find Keyboard row and click it.
	if err := ui.DoDefault(nodewith.Role(role.Link).NameStartingWith("Keyboard and inputs"))(ctx); err != nil {
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

	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for the screen to be locked failed: %v (last status %+v)", err, st)
	}

	// Unlock the screen to ensure subsequent tests aren't affected by the screen remaining locked.
	// TODO(b/187794615): Remove once chrome.go has a way to clean up the lock screen state.
	defer func() {
		if err := lockscreen.Unlock(cleanupCtx, tconn); err != nil {
			s.Fatal("Failed to unlock the screen: ", err)
		}
	}()

	if err = lockscreen.WaitForPasswordField(ctx, tconn, username, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for password field: ", err)
	}

	field, err := lockscreen.PasswordFieldFinder(username)
	if err != nil {
		s.Fatal("Failed to find password field: ", err)
	}

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
	if err := kb.Type(ctx, password); err != nil {
		s.Fatal("Failed to type password: ", err)
	}

	if err := lockscreen.ShowPassword(ctx, tconn); err != nil {
		s.Fatal("Failed to click the Show password button: ", err)
	}

	passwordField, err := lockscreen.UserPassword(ctx, tconn, username, false)
	if err != nil {
		s.Fatal("Failed to read Password: ", err)
	}

	if passwordField.Value != password {
		s.Fatalf("Passwords do not match Password Field: %q User entered value: %q", passwordField.Value, password)
	}

	if err := kb.Accel(ctx, "Ctrl"); err != nil {
		s.Fatal("Failed to press Ctrl: ", err)
	}

	passwordField, err = lockscreen.UserPassword(ctx, tconn, username, false)
	if err != nil {
		s.Fatal("Failed to read Password: ", err)
	}

	if passwordField.Value == password {
		s.Fatal("Passwords unexpectedly match. Remapping Ctrl -> Backspace did not persist to login screen settings")
	}
}
