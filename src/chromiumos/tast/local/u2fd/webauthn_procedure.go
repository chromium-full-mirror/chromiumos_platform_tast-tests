// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package u2fd contains functionality shared by WebAuthn/U2F tests.
package u2fd

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// Below functions of the individual procedures of MakeCredential/GetAssertion
// are exported because some tests utilize the individual steps, possibly doing
// some extra steps in between, or not executing all of them.

// WaitUntilPopupGone waits until the power button press prompt is gone.
func WaitUntilPopupGone(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	popupMessageNode := nodewith.ClassName("MessagePopupView")
	if err := ui.WaitUntilGone(popupMessageNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for power button press prompt gone")
	}
	// EnsureGoneFor is needed because the caller might summon the popup more than once.
	// The purpose of this function is to wait until the ongoing state of previous
	// operation is ended.
	if err := ui.EnsureGoneFor(popupMessageNode, 3*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure power button press prompt gone for a while")
	}
	return nil
}

// WaitForPopup waits until the power button press prompt appears.
func WaitForPopup(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	popupMessageNode := nodewith.ClassName("MessagePopupView")
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(popupMessageNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for power button press prompt")
	}
	return nil
}

// ChoosePlatformAuthenticator chooses the platform authenticator ("This device")
// option when Chrome prompts for authenticator selection during WebAuthn requests.
func ChoosePlatformAuthenticator(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	platformAuthenticatorButton := nodewith.Role(role.Button).Name("This device")
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(platformAuthenticatorButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to select platform authenticator from transport selection sheet")
	}
	if err := ui.DoDefault(platformAuthenticatorButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click button for platform authenticator")
	}
	return nil
}

// WaitForWebAuthnDialog waits for the in-session WebAuthn dialog that
// should be prompted when a user-verification WebAuthn request is sent
// to the platform authenticator.
func WaitForWebAuthnDialog(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	dialog := nodewith.ClassName("AuthDialogWidget")
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(dialog)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the ChromeOS dialog")
	}
	return nil
}
