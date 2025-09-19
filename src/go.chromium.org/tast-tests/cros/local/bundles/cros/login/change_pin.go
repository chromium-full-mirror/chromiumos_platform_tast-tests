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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/auth"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChangePin,
		Desc: "Checks pin change flow in OS Settings for Pin only user",
		Contacts: []string{
			"cros-lurs@google.com",
			"iscsi@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 2*time.Minute,
	})
}

func ChangePin(ctx context.Context, s *testing.State) {
	const (
		username = "testuser@gmail.com"
		oldPin   = "123456"
		newPin   = "654321"
	)

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer userutil.ResetUsers(cleanupContext)

	func() {
		cr, err := login.SetupUserWithPin(ctx,
			oldPin,
			chrome.FakeLogin(chrome.Creds{User: username, Pass: ""}),
			chrome.ExtraArgs("--disable-first-run-ui"),
		)
		if err != nil {
			s.Fatal("Failed to setup user: ", err)
		}
		defer cr.Close(cleanupContext)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Getting test API connection failed: ", err)
		}

		// Open OS Settings > lock screen.
		settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy/lockScreen", func(context.Context) error { return nil })
		if err != nil {
			s.Fatal("Failed to open setting page: ", err)
		}
		defer settings.Close(cleanupContext)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_dump")

		// The page is pin protected, confirm the old pin.
		if err := auth.ConfirmPin(ctx, cr, oldPin, true /*pin_only*/); err != nil {
			s.Fatal("Failed to confirm pin: ", err)
		}

		if err := changePinInSettings(ctx, cleanupContext, tconn, newPin); err != nil {
			s.Fatal("Failed to change pin: ", err)
		}

		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupContext)

	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_dump")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(cleanupContext)

	if err := lockscreen.EnterPIN(ctx, tLoginConn, keyboard, newPin); err != nil {
		s.Fatal("Failed to enter pin: ", err)
	}

	// Check if the login was successful using the API and also by looking for the shelf in the UI.
	if err := lockscreen.WaitForLoggedIn(ctx, tLoginConn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	if err := ash.WaitForShelf(ctx, tLoginConn, 30*time.Second); err != nil {
		s.Fatal("Shelf did not appear after logging in: ", err)
	}
}

// changePinInSettings changes the pin on
// the lock screen page in OS Settings (the page should be already open).
func changePinInSettings(ctx, cleanupContext context.Context, tconn *chrome.TestConn, pin string) error {
	ui := uiauto.New(tconn)

	changePinButton := nodewith.Name("Change PIN").Role(role.Button)
	dialog := nodewith.Name("Enter your PIN").Role(role.Dialog)
	continueButton := nodewith.Name("Continue").Role(role.Button)
	confirmButton := nodewith.Name("Confirm").Role(role.Button)
	confirmDialog := nodewith.Name("Confirm your PIN").Role(role.Dialog)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(cleanupContext)

	if err := uiauto.Combine("change pin",
		ui.WaitUntilExists(changePinButton),
		ui.LeftClick(changePinButton),
		ui.WaitUntilExists(dialog),
		keyboard.TypeAction(pin), // Enter pin.
		ui.LeftClick(continueButton),
		ui.WaitUntilExists(confirmDialog),
		keyboard.TypeAction(pin), // Confirm pin.
		ui.LeftClick(confirmButton),
		ui.WaitUntilGone(confirmDialog),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to change pin")
	}

	return nil
}
