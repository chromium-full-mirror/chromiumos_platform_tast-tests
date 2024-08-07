// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package login provides functions for user login and auth factors setup
package login

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SetupUserWithLocalPassword starts chrome, navigates to local password setup
// screen in OOBE and sets the password.
// The user should be provided in `opts` (e.g. using `chrome.FakeLogin`,
// `chrome.GAIALogin`).
func SetupUserWithLocalPassword(ctx context.Context, password string, opts ...chrome.Option) (c *chrome.Chrome, retErr error) {
	opts = append(opts, chrome.DisableFeatures("CryptohomeRecoveryBeforeFlowSplit"))
	opts = append(opts, chrome.DontSkipOOBEAfterLogin())

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return cr, errors.Wrap(err, "failed to wait for OOBE connection")
	}
	defer oobeConn.Close()

	if err := SetupLocalPassword(ctx, oobeConn, password); err != nil {
		return cr, errors.Wrap(err, "failed to setup local password")
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.skipPostLoginScreens()", nil); err != nil {
		// This is not fatal because sometimes it fails because Oobe shutdowns too
		// fast after the call - which produces error.
		testing.ContextLog(ctx, "Failed to call skip post login screens: ", err)
	}
	if err := cr.WaitForOOBEConnectionToBeDismissed(ctx); err != nil {
		return cr, errors.Wrap(err, "failed to wait for OOBE to be dismissed")
	}

	return cr, nil
}

// SetupUserWithLocalPasswordAndPin starts chrome, navigates to local password
// setup screen in OOBE and sets the password. After that navigates to the pin
// setup screen and sets the PIN. The user should be provided in `opts`
// (e.g. using `chrome.FakeLogin`, `chrome.GAIALogin`).
func SetupUserWithLocalPasswordAndPin(ctx context.Context, password, pin string, opts ...chrome.Option) (c *chrome.Chrome, retErr error) {
	opts = append(opts, chrome.DisableFeatures("CryptohomeRecoveryBeforeFlowSplit"))
	opts = append(opts, chrome.DontSkipOOBEAfterLogin())

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		return cr, errors.Wrap(err, "failed to wait for OOBE connection")
	}
	defer oobeConn.Close()

	if err := SetupLocalPassword(ctx, oobeConn, password); err != nil {
		return cr, errors.Wrap(err, "failed to setup local password")
	}

	if err := SetupPin(ctx, oobeConn, pin); err != nil {
		return cr, errors.Wrap(err, "failed to setup pin")
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.skipPostLoginScreens()", nil); err != nil {
		// This is not fatal because sometimes it fails because Oobe shutdowns
		// too fast after the call - which produces error.
		testing.ContextLog(ctx, "Failed to call skip post login screens: ", err)
	}
	if err := cr.WaitForOOBEConnectionToBeDismissed(ctx); err != nil {
		return cr, errors.Wrap(err, "failed to wait for OOBE to be dismissed")
	}

	return cr, nil
}

// SetupLocalPassword navigates to the local password setup screen in OOBE and
// submits provided password.
func SetupLocalPassword(ctx context.Context, oobeConn *chrome.Conn, password string) error {
	// After the Gaia login we need to navigate to the local password setup screen
	// to set the local password (instead of the Gaia password).
	// TODO(b/309740812): Replace the OOBE calls with one API to setup a local
	// password.
	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('local-password-setup')", nil); err != nil {
		return errors.Wrap(err, "failed to advance to the 'local-password-setup' screen")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.LocalPasswordSetupScreen.isReadyForTesting()"); err != nil {
		return errors.Wrap(err, "failed to wait for the LocalPasswordSetupScreen to be visible")
	}
	if err := oobeConn.Call(ctx, nil, `(pw) => { OobeAPI.screens.LocalPasswordSetupScreen.enterPasswordToFirstInput(pw); }`, password); err != nil {
		return errors.Wrap(err, "failed to enter local password")
	}
	if err := oobeConn.Call(ctx, nil, `(pw) => { OobeAPI.screens.LocalPasswordSetupScreen.enterPasswordToConfirmInput(pw); }`, password); err != nil {
		return errors.Wrap(err, "failed to enter local password")
	}
	if err := oobeConn.WaitForExprWithTimeout(ctx, "OobeAPI.screens.LocalPasswordSetupScreen.nextButton.isEnabled()", 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for the next button being enabled")
	}
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.LocalPasswordSetupScreen.nextButton.click()", nil); err != nil {
		return errors.Wrap(err, "failed to click on the next button")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.isDone()"); err != nil {
		return errors.Wrap(err, "failed to wait for the done step to be visible")
	}
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.clickDone()", nil); err != nil {
		return errors.Wrap(err, "failed to click on done button")
	}

	return nil
}

// SetupPin function navigates to the pin setup screen in OOBE and
// submits provided PIN.
func SetupPin(ctx context.Context, oobeConn *chrome.Conn, pin string) error {
	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('pin-setup')", nil); err != nil {
		return errors.Wrap(err, "failed to advance to the pin screen")
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "!document.querySelector('#pin-setup').hidden"); err != nil {
		return errors.Wrap(err, "failed to wait for the pin screen")
	}

	for _, step := range []string{"start", "confirm"} {
		if err := oobeConn.WaitForExprFailOnErr(ctx, fmt.Sprintf("document.querySelector('#pin-setup').uiStep === '%s'", step)); err != nil {
			return errors.Wrap(err, "failed to wait pin setup step")
		}
		if err := oobeConn.Eval(ctx, fmt.Sprintf("document.querySelector('#pin-setup').$.pinKeyboard.$.pinKeyboard.$.pinInput.value = '%s'", pin), nil); err != nil {
			return errors.Wrap(err, "failed to enter pin")
		}

		if err := oobeConn.Eval(ctx, "document.querySelector('#pin-setup').$.nextButton.click()", nil); err != nil {
			return errors.Wrap(err, "failed to click on the next button")
		}
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "document.querySelector('#pin-setup').uiStep === 'done'"); err != nil {
		return errors.Wrap(err, "failed to wait for the done step")
	}

	return nil
}
