// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package auth provides functions that help with testing of the
// in session authentication CUJs.
package auth

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast/core/errors"
)

// Elements of the settings authentication dialog.
var (
	// The Active Session Auth approach widget name.
	ActiveSessionWidget = nodewith.ClassName("AuthDialogWidget")
)

// ConfiguredAuthType represents the configured authentication factor(s) during OOBE.
type ConfiguredAuthType int

// SetupWithPassword - Sets up password factor for the test user during OOBE.
// SetupWithPasswordAndPin - Sets up password and PIN factors for the test user during OOBE.
// SetupWithPin - Sets up PIN as only factor for the test user during OOBE.
const (
	SetupWithPassword ConfiguredAuthType = iota
	SetupWithPasswordAndPin
	SetupWithPin
)

// InSessionAuthType represents the used authentication factor to enter the authentication requested page.
type InSessionAuthType int

// AuthWithPassword - Authenticate the test user with password when in session dialog pops up.
// AuthWithPin - Authenticate the test user with PIN when in session dialog pops up.
// AuthCancel - Cancel the authentication of the test user when in session dialog pops up.
const (
	AuthWithPassword InSessionAuthType = iota
	AuthWithPin
	AuthCancel // this option is closing the authentication widget.
)

// InSessionParam provide configuration for in session authentication tests.
type InSessionParam struct {
	ConfiguredAuth ConfiguredAuthType
	InSessionAuth  InSessionAuthType
}

// SetupUser configures a new chrome with the provided user credentials
func SetupUser(ctx context.Context, params InSessionParam, username, password, pin string) (*chrome.Chrome, error) {

	loginOption := chrome.FakeLogin(chrome.Creds{User: username, Pass: ""})
	chromeArgs := chrome.ExtraArgs("--disable-first-run-ui")

	switch params.ConfiguredAuth {
	case SetupWithPassword:
		return login.SetupUserWithLocalPassword(ctx, password,
			loginOption,
			chromeArgs)
	case SetupWithPasswordAndPin:
		return login.SetupUserWithLocalPasswordAndPin(ctx, password, pin,
			loginOption,
			chromeArgs)
	case SetupWithPin:
		return login.SetupUserWithPin(ctx, pin, loginOption, chromeArgs)
	}
	return nil, errors.New("invalid setup type")
}

// ConfirmPassword enters the provided password in session authentication dialog,
// to open authentication protected pages.
func ConfirmPassword(ctx context.Context, cr *chrome.Chrome, password string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	uia := uiauto.New(tconn)
	if err := uia.WaitUntilAnyExists(ActiveSessionWidget)(ctx); err != nil {
		return errors.Wrap(err, "failed to find password dialog")
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	if err := keyboard.Type(ctx, password+"\n"); err != nil {
		return errors.Wrap(err, "failed to type password")
	}

	if err := uia.WaitUntilGone(ActiveSessionWidget)(ctx); err != nil {
		return errors.Wrap(err, "ActiveSessionWidget is still present after entering password")
	}

	return nil
}

// ConfirmPin authenticates user with PIN in session dialog
// This function enables access to authentication-protected pages for users
// configured with a PIN. First it switches the authentication dialog to the
// PIN input mode, then types the provided PIN, and finally submits it to
// complete the authentication process.
func ConfirmPin(ctx context.Context, cr *chrome.Chrome, pin string, pinOnly bool) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	uia := uiauto.New(tconn)
	if err := uia.WaitUntilExists(ActiveSessionWidget)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the ActiveSessionWidget")
	}

	if !pinOnly {
		// Clicking the switch to PIN button.
		switchToPinButton := nodewith.Name("Switch to PIN").ClassName("PillButton")

		if err := uia.WaitUntilExists(switchToPinButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the 'Switch to PIN' button")
		}

		if err := uia.LeftClick(switchToPinButton)(ctx); err != nil {
			errors.Wrap(err, "failed to click the switch to pin button")
		}
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	if err := keyboard.Type(ctx, pin+"\n"); err != nil {
		return errors.Wrap(err, "failed to type pin")
	}

	if err := uia.WaitUntilGone(ActiveSessionWidget)(ctx); err != nil {
		return errors.Wrap(err, "ActiveSessionWidget is still present after entering pin")
	}

	return nil
}

// CancelPassword cancels out of a password prompt in session authentication dialog.
func CancelPassword(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	uia := uiauto.New(tconn)
	if err := uia.WaitUntilAnyExists(ActiveSessionWidget)(ctx); err != nil {
		return errors.Wrap(err, "failed to find password dialog")
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	// The simplest way to cancel out is via an ESC.
	if err := keyboard.Type(ctx, "\x1b"); err != nil {
		return errors.Wrap(err, "failed to hit ESC")
	}

	if err := uia.WaitUntilGone(ActiveSessionWidget)(ctx); err != nil {
		return errors.Wrap(err, "ActiveSessionWidget is still present after cancelling")
	}

	return nil
}
