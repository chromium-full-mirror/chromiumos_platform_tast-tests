// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangeLocalPassword,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks local password change flow in OS Settings",
		Contacts: []string{
			"cros-lurs@google.com",
			"anastasiian@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

func ChangeLocalPassword(ctx context.Context, s *testing.State) {
	const (
		username    = "testuser@gmail.com"
		oldPassword = "oldtestpassword"
		newPassword = "newtestpassword"
	)

	func() {
		cr, err := login.SetupUserWithLocalPassword(ctx,
			oldPassword,
			// We don't need to provide Gaia password, as the local password will be
			// used instead.
			chrome.FakeLogin(chrome.Creds{User: username, Pass: ""}),
		)
		if err != nil {
			s.Fatal("Failed to setup user: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Getting test API connection failed: ", err)
		}

		// Open OS Settings > lock screen.
		settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy/lockScreen", func(context.Context) error { return nil })
		if err != nil {
			s.Fatal("Failed to open setting page: ", err)
		}
		defer settings.Close(ctx)
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

		// The page is password protected, confirm the old local password.
		if err := ossettings.ConfirmPassword(ctx, cr, oldPassword); err != nil {
			s.Fatal("Failed to confirm password: ", err)
		}

		if err := changeLocalPasswordInSettings(ctx, tconn, newPassword); err != nil {
			s.Fatal("Failed to change local password: ", err)
		}

		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.EnableFeatures("LocalPasswordForConsumers"),
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tLoginConn)

	// Wait for the login screen to be ready for password entry.
	if err := lockscreen.WaitForPasswordEntry(ctx, tLoginConn, 30*time.Second); err != nil {
		s.Fatal("Failed waiting for the login screen to be ready for password entry: ", err)
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	s.Log("Entering password to log in")
	if err := lockscreen.EnterPassword(ctx, tLoginConn, username, newPassword, keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	// Check if the login was successful using the API and also by looking for the shelf in the UI.
	if err := lockscreen.WaitForLoggedIn(ctx, tLoginConn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	if err := ash.WaitForShelf(ctx, tLoginConn, 30*time.Second); err != nil {
		s.Fatal("Shelf did not appear after logging in: ", err)
	}
}

// changeLocalPasswordInSettings changes the local password to `password` on
// the lock screen page in OS Settings (the page should be already open).
func changeLocalPasswordInSettings(ctx context.Context, tconn *chrome.TestConn, password string) error {
	ui := uiauto.New(tconn)

	changePasswordButton := nodewith.Name("Change password").Role(role.Button)
	dialog := nodewith.Name("Set device password").Role(role.Dialog)
	passwordField := nodewith.Name("Password").Role(role.TextField)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	if err := uiauto.Combine("change password",
		ui.WaitUntilExists(changePasswordButton),
		ui.LeftClick(changePasswordButton),
		ui.WaitUntilExists(dialog),
		ui.LeftClick(passwordField),
		keyboard.TypeAction(password+"\n"), // Enter password.
		keyboard.TypeAction(password+"\n"), // Confirm password.
		ui.WaitUntilGone(dialog),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to change password")
	}

	return nil
}
