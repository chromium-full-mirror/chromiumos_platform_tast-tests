// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ms365 contains the page object to interact with Microsoft login
// page and the Microsoft 365 app window.
package ms365

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	officePWAInstallURL = "https://www.microsoft365.com/?from=Homescreen"
)

func log(msg string) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, msg)
		return nil
	}
}

// Ms365 represents an instance of the Microsoft 365 app UI.
type Ms365 struct {
	ui       *uiauto.Context
	kb       *input.KeyboardEventWriter
	tconn    *chrome.TestConn
	UserName string
	Password string
}

// App returns an instance of the Cloud Upload.
func App(ctx context.Context, tconn *chrome.TestConn, accountPool string) (*Ms365, error) {
	// Create a uiauto.Context with default timeout.
	ui := uiauto.New(tconn).WithInterval(500 * time.Millisecond)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}

	msCreds, err := credconfig.PickRandomCreds(accountPool)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the user/passwd for Microsoft 365")
	}

	return &Ms365{ui: ui, kb: kb, tconn: tconn, UserName: msCreds.User, Password: msCreds.Pass}, nil
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

// StaySignedIn waits for the Microsoft "Stayed Signed in?" screen and clicks YES.
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

// LoginToMicrosoft365 logs into the Microsoft 365 app with the username and password in the instance.
// If the account is logged in before, the auth flow will skip password screen, "skipPassword" flag is used to control that.
func (ms *Ms365) LoginToMicrosoft365(setupCompleteDialogFinder *nodewith.Finder, skipPassword bool) uiauto.Action {
	return uiauto.Combine("Login to Microsoft 365",
		ms.InputUserName(ms.UserName),
		func(ctx context.Context) error {
			if skipPassword {
				log("Skipping password")(ctx)
				return nil
			}
			return uiauto.Combine("Input password and stay signed in",
				ms.InputPassword(ms.Password),
				ms.StaySignedIn(),
			)(ctx)
		},
		ms.AcceptPermissionIfNeeded(setupCompleteDialogFinder),
	)
}

// Microsoft365WindowFinder is a finder for a window of Microsoft 365 (Word, Excel or PowerPoint)
// opened for the given fileName.
func Microsoft365WindowFinder(fileName string) *nodewith.Finder {
	return nodewith.Role(role.Window).NameContaining(fileName).HasClass("BrowserRootView")
}

// WaitForMicrosoft365Window wait for the Microsoft 365 window with the
// specified file name/type in the title to open.
func (ms *Ms365) WaitForMicrosoft365Window(fileName string) uiauto.Action {
	ms365App := Microsoft365WindowFinder(fileName)
	return ms.ui.WaitUntilExists(ms365App)
}

// InstallPWA installs Office PWA.
func (ms *Ms365) InstallPWA(ctx context.Context, cr *chrome.Chrome, browserType browser.Type) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	_, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, officePWAInstallURL)
	if err != nil {
		return errors.Wrap(err, "failed to launch browser")
	}
	defer cleanup(cleanupCtx)

	// Installing Office PWA requires a valid login.
	signInButton := nodewith.Role(role.Button).Name("Sign in")
	if err := uiauto.Combine("Login to Office site",
		ms.ui.WaitUntilExists(signInButton),
		ms.ui.LeftClick(signInButton),
		ms.InputUserName(ms.UserName),
		ms.InputPassword(ms.Password),
		ms.StaySignedIn(),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to login to Office site")
	}

	windowAfterSignIn := nodewith.Role(role.RootWebArea).Name("Home | Microsoft 365")
	installIcon := nodewith.ClassName("PwaInstallView").Role(role.Button)
	installButton := nodewith.Name("Install").Role(role.Button)

	if err := uiauto.Combine("Install Office PWA through omnibox",
		ms.ui.WaitUntilExists(windowAfterSignIn),
		ms.ui.WithTimeout(30*time.Second).WaitUntilExists(installIcon),
		ms.ui.LeftClick(installIcon),
		// The popup containing Install button takes time to appear sometimes.
		ms.ui.WithTimeout(time.Minute).WaitUntilExists(installButton),
		ms.ui.LeftClick(installButton))(ctx); err != nil {
		return errors.Wrap(err, "failed to install Office PWA")
	}
	return ash.WaitForChromeAppInstalled(ctx, ms.tconn, apps.Microsoft365.ID, time.Minute)
}

// ClearBrowserCookiesForOffice will clear all browser cookies for the Office website.
func ClearBrowserCookiesForOffice(ctx context.Context, cr *chrome.Chrome) error {
	log("Clearing cookies for Office website")(ctx)
	br := cr.Browser()
	conn, err := br.NewTab(ctx, officePWAInstallURL)
	if err != nil {
		return errors.Wrap(err, "failed to open office website")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	return conn.ClearSiteCookies(ctx, "www.microsoft365.com")
}
