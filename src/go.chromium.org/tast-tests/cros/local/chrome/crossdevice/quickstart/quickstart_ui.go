// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quickstart is for controlling ChromeOS Quick Start functionality.
package quickstart

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/errors"
)

// SelectForPersonalUse waits for the User Creation Screen to appear and
// selects the "For personal use" option.
func SelectForPersonalUse(ctx context.Context, oobeConn *chrome.Conn, ui *uiauto.Context) error {
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.UserCreationScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the user creation screen to be visible")
	}

	personalUseButton := nodewith.NameContaining("personal use").Role(role.RadioButton)
	if err := ui.LeftClick(personalUseButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the personal Google Account radio button")
	}

	nextButton := nodewith.Name("Next").Role(role.Button)
	if err := ui.LeftClick(nextButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click Next on the user creation screen")
	}

	return nil
}

// VerifyPIN extracts the verification PIN from each device and ensures they match.
func VerifyPIN(ctx context.Context, ui *uiauto.Context, androidDevice *crossdevice.AndroidDevice) error {
	phonePIN, err := androidDevice.ExtractQuickStartVerificationPIN(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to extract the verification PIN from the phone")
	}

	// Ensure each digit from the PIN on the phone side appears on the Chromebook
	// screen. This is close enough given the limitations of the HTML on this
	// screen.
	for _, digit := range phonePIN {
		digitNode := nodewith.Name(string(digit)).Role(role.StaticText).First()
		if err := ui.WaitUntilExists(digitNode)(ctx); err != nil {
			return errors.Wrap(err, "PIN on Chromebook does not match")
		}
	}

	return nil
}
