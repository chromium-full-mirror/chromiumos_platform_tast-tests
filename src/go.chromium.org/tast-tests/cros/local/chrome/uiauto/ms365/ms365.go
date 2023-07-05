// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ms365 contains the page object to interact with Microsoft login
// page and the Microsoft 365 app window.
package ms365

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// Ms365 represents an instance of the Microsoft 365 app UI.
type Ms365 struct {
	ui    *uiauto.Context
	kb    *input.KeyboardEventWriter
	tconn *chrome.TestConn
}

// App returns an instance of the Cloud Upload.
func App(ctx context.Context, tconn *chrome.TestConn) (*Ms365, error) {
	// Create a uiauto.Context with default timeout.
	ui := uiauto.New(tconn).WithInterval(500 * time.Millisecond)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}

	return &Ms365{ui: ui, kb: kb, tconn: tconn}, nil
}

// InputUserName waits for the Microsoft sign in window and input the username.
func (ms *Ms365) InputUserName(userName string) uiauto.Action {
	msSignInWindow := nodewith.Role(role.RootWebArea).Name("Sign in to your account")
	usernameInput := nodewith.Ancestor(msSignInWindow).Role(role.TextField).NameContaining("email")

	return uiauto.Combine("MS SignIn",
		ms.ui.WaitUntilExists(msSignInWindow),
		ms.ui.LeftClick(usernameInput),
		ms.kb.TypeAction(userName),
		ms.kb.AccelAction("Enter"),
	)
}

// InputPassword waits for the Microsoft "input password" screen and input the password.
func (ms *Ms365) InputPassword(password string) uiauto.Action {
	msPasswordWindow := nodewith.Role(role.RootWebArea).Name("Sign in to your Microsoft account")
	passwordInput := nodewith.Ancestor(msPasswordWindow).Role(role.TextField).NameContaining("password")

	return uiauto.Combine("MS SignIn Password",
		ms.ui.WaitUntilExists(msPasswordWindow),
		ms.ui.WaitUntilExists(passwordInput),
		ms.ui.LeftClick(passwordInput),
		ms.kb.TypeAction(password),
		ms.kb.AccelAction("Enter"),
	)
}

// StaySignedIn waits for the Microsoft "Styed Signed in?" screen and clicks
// YES.
func (ms *Ms365) StaySignedIn() uiauto.Action {
	msStaySignedInWindow := nodewith.Role(role.RootWebArea).Name("Microsoft account")
	msStaySignedInButton := nodewith.Ancestor(msStaySignedInWindow).Role(role.Button).Name("Yes")

	return uiauto.Combine("MS Stay Signed In",
		ms.ui.WaitUntilExists(msStaySignedInButton),
		ms.ui.LeftClick(msStaySignedInButton),
	)
}

// AcceptPermissionIfNeeded waits for the optional Microsoft "Access permission" screen and clicks to accept it.
// It also detects the cloud_upload.SetupComplete dialog to indicate that the permission screen wasn't presented at all.
func (ms *Ms365) AcceptPermissionIfNeeded(setupCompleteDialogFinder *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		msAcceptPermissionWindow := nodewith.Role(role.RootWebArea).NameContaining("Let this app access your info?")
		msAcceptPermissionButton := nodewith.Ancestor(msAcceptPermissionWindow).Role(role.Button).Name("Accept")
		runAcceptPermission := uiauto.Combine("MS permission screen",
			ms.ui.WaitUntilExists(msAcceptPermissionButton),
			ms.kb.TypeKeyAction(input.KEY_END),
			ms.ui.LeftClick(msAcceptPermissionButton),
		)

		// When the permission dialog doesn't show it goes directly to Setup Complete dialog.
		found, err := ms.ui.FindAnyExists(ctx, msAcceptPermissionWindow, setupCompleteDialogFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find the MS accept permission window")
		}
		if found == msAcceptPermissionWindow {
			return runAcceptPermission(ctx)
		}
		// `setupCompleteDialog` has been found, nothing to do.
		return nil

	}
}

// LoginToMicrosoft365 logs into the Microsoft 365 app with the provided
// username and password.
func (ms *Ms365) LoginToMicrosoft365(userName, password string, setupCompleteDialogFinder *nodewith.Finder) uiauto.Action {
	return uiauto.Combine("Login to Microsoft 365",
		ms.InputUserName(userName),
		ms.InputPassword(password),
		ms.StaySignedIn(),
		ms.AcceptPermissionIfNeeded(setupCompleteDialogFinder),
	)
}

// WaitForMicrosoft365Window wait for the Microsoft 365 window with the
// specified file name/type in the title to open.
func (ms *Ms365) WaitForMicrosoft365Window(fileName string) uiauto.Action {
	ms365App := nodewith.Role(role.Window).NameContaining(fileName).HasClass("BrowserRootView")
	// TODO: Increase timeout here? It failed once.
	return ms.ui.WaitUntilExists(ms365App)
}
