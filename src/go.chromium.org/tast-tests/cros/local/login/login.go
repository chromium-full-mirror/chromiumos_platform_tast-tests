// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package login provides functions for user login and auth factors setup
package login

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

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
	if err := oobeConn.Call(ctx, nil, `(pw) => { OobeAPI.screens.LocalPasswordSetupScreen.enterPassword(pw); }`, password); err != nil {
		return errors.Wrap(err, "failed to enter local password")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.isDone()"); err != nil {
		return errors.Wrap(err, "failed to wait for the done step to be visible")
	}
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.clickDone()", nil); err != nil {
		return errors.Wrap(err, "failed to click on done button")
	}

	return nil
}
