// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package signinutil provides functions that help with management of sign-in restrictions
package signinutil

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// RestrictSignInOption is the name of the option in OS Settings that allows user to
// restrict sign-in to the existing or provided list of users.
const RestrictSignInOption = "Restrict sign-in to the following users:"

// GetUsernameFromEmail returns the part of the email before '@'.
func GetUsernameFromEmail(email string) string {
	return email[:strings.IndexByte(email, '@')]
}

// OpenManageOtherPeople opens "Manage other people" section in OS Settings.
func OpenManageOtherPeople(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*ossettings.OSSettings, error) {
	ui := uiauto.New(tconn)

	const subsettingsName = "Manage other people"

	// Open settings, Manage Other People.
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy", ui.WaitUntilExists(nodewith.Name(subsettingsName)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the settings page")
	}

	if err := ui.LeftClick(nodewith.Name(subsettingsName))(ctx); err != nil {
		return settings, errors.Wrap(err, "failed to open Manage other people subsettings")
	}

	if err := ui.WaitUntilExists(nodewith.Name(RestrictSignInOption).Role(role.ToggleButton))(ctx); err != nil {
		return settings, errors.Wrap(err, "failed to wait for the toggle to show the list of users")
	}

	return settings, nil
}

// EnterInvalidPassword enters invalid password on the login screen and waits until
// the auth error dialog is shown.
func EnterInvalidPassword(ctx context.Context, cr *chrome.Chrome, creds chrome.Creds) error {
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create login test API connection")
	}

	// Wait for the login screen to be ready for password entry.
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.ReadyForPassword }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "failed waiting for the login screen to be ready for password entry last state %+v", st)
	}

	// TODO(crbug/1109381): the password field isn't actually ready just yet when WaitState returns.
	// This causes it to miss some of the keyboard input, so the password will be wrong.
	// We can check in the UI for the password field to exist, which seems to be a good enough indicator that
	// the field is ready for keyboard input.
	if err := lockscreen.WaitForPasswordField(ctx, tconn, creds.User, 5*time.Second); err != nil {
		return errors.Wrap(err, "password text field did not appear in the UI")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get virtual keyboard")
	}
	defer keyboard.Close(ctx)

	// Enter wrong password.
	if err = lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass+"invalid", keyboard); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}
	if err := lockscreen.WaitForAuthError(ctx, tconn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for auth error")
	}

	return nil
}

// WaitForRecoverySetup navigates to the recovery setup screen and waits until
// it's not shown anymore.
func WaitForRecoverySetup(ctx context.Context, oobeConn *chrome.Conn) error {
	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('cryptohome-recovery-setup')", nil); err != nil {
		return errors.Wrap(err, "failed to advance to the TOS screen")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.CryptohomeRecoverySetupScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the recovery setup screen to be visible")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "!OobeAPI.screens.CryptohomeRecoverySetupScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the recovery setup screen to not be visible")
	}

	return nil
}
