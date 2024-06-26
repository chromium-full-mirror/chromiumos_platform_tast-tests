// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ms365 contains the page object to interact with Microsoft login
// page and the Microsoft 365 app window.
package ms365

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	officePWAInstallURL = "https://www.microsoft365.com/?from=Homescreen"
)

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
	// Most of the interactions are with remote service, increase the timeout to 60s.
	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

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

// AppWithCreds returns an instance of the M365 with defined credentials.
func AppWithCreds(ctx context.Context, tconn *chrome.TestConn, creds credconfig.Creds) (*Ms365, error) {
	// Most of the interactions are with remote service, increase the timeout to 60s.
	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}

	return &Ms365{ui: ui, kb: kb, tconn: tconn, UserName: creds.User, Password: creds.Pass}, nil
}

// InputUserName waits for the Microsoft sign in window and input the username.
func (ms *Ms365) InputUserName(userName string) uiauto.Action {
	msSignInWindow := nodewith.Role(role.RootWebArea).Name("Sign in to your account")
	usernameInput := nodewith.Ancestor(msSignInWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(email|someone@example.com).*"))

	return uiauto.Combine("MS SignIn",
		ms.ui.WaitUntilExists(msSignInWindow),
		ms.ui.WaitUntilExists(usernameInput.Visible()),
		ms.ui.LeftClickUntilFocused(usernameInput),
		ms.kb.TypeAction(userName),
		ms.kb.AccelAction("Enter"),
		ms.ui.WaitUntilGone(usernameInput.Visible()),
	)
}

// InputPassword waits for the Microsoft "input password" screen and input the password.
func (ms *Ms365) InputPassword(password string) uiauto.Action {
	msPasswordWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("Sign in to your( Microsoft)? account"))
	passwordInput := nodewith.Ancestor(msPasswordWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(p|P)assword.*"))

	return uiauto.Combine("MS SignIn Password",
		ms.ui.WaitUntilExists(msPasswordWindow.Visible()),
		ms.ui.WaitUntilExists(passwordInput.Visible()),
		ms.ui.LeftClickUntilFocused(passwordInput),
		ms.kb.TypeAction(password),
		ms.kb.AccelAction("Enter"),
		ms.ui.WaitUntilGone(passwordInput.Visible()),
	)
}

// StaySignedIn waits for the Microsoft "Stayed Signed in?" screen and clicks YES.
func (ms *Ms365) StaySignedIn() uiauto.Action {
	msStaySignedInWindowName := regexp.MustCompile("(Microsoft account|Sign in to your account|" + regexp.QuoteMeta("Stay signed in?") + ")")
	msStaySignedInWindow := nodewith.Role(role.RootWebArea).NameRegex(msStaySignedInWindowName)
	msStaySignedInButton := nodewith.Ancestor(msStaySignedInWindow).Role(role.StaticText).Name("Yes")

	return uiauto.Combine("MS Stay Signed In",
		ms.ui.WaitUntilExists(msStaySignedInButton),
		ms.ui.LeftClickUntil(msStaySignedInButton, ms.ui.Gone(msStaySignedInButton)),
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
			ms.ui.LeftClickUntil(msAcceptPermissionButton, ms.ui.Gone(msAcceptPermissionButton)),
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
				testing.ContextLog(ctx, "Skipping password")
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
	return nodewith.Role(role.Window).NameContaining(fileName).HasClass("WebContentsViewAura")
}

// closeMicrosoft365Window finds the Microsoft 365 app window with the specific
// file name and close it.
func closeMicrosoft365Window(ctx context.Context, tconn *chrome.TestConn, fileName string) error {
	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
		return strings.Contains(w.Title, fileName) && strings.Contains(w.Title, "Microsoft")
	})
	if err != nil {
		return errors.Wrap(err, "failed to find the MS365 window to close")
	}
	return w.CloseWindow(ctx, tconn)
}

// CloseMicrosoftAuthWindowWithoutSignIn finds the Microsoft auth window and
// close it without signing in.
func CloseMicrosoftAuthWindowWithoutSignIn(tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "Sign in to your account")
		if err != nil {
			return errors.Wrap(err, "failed to find the MS365 auth window to close")
		}
		return w.CloseWindow(ctx, tconn)
	}
}

// waitForMicrosoft365WindowAndClose waits for the Microsoft 365 app window to open and close it.
// If "ignoreCloseError" is true, it will just ignore the error, otherwise it returns error if the closure fails,.
func (ms *Ms365) waitForMicrosoft365WindowAndClose(tconn *chrome.TestConn, fileName string, ignoreCloseError bool) uiauto.Action {
	return func(ctx context.Context) error {
		ms365App := Microsoft365WindowFinder(fileName)
		if err := ms.ui.WaitUntilExists(ms365App)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for the MS365 window")
		}
		if err := closeMicrosoft365Window(ctx, tconn, fileName); err != nil {
			if !ignoreCloseError {
				return errors.Wrap(err, "failed to close the MS365 window")
			}
		}
		return nil
	}
}

// WaitForMicrosoft365WindowAndClose closes the MS window and guarantees that
// the window is closed successfully, it will return errors if the closure fails.
func (ms *Ms365) WaitForMicrosoft365WindowAndClose(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return ms.waitForMicrosoft365WindowAndClose(tconn, fileName, false /*=ignoreCloseError*/)
}

// WaitForMicrosoft365WindowAndCloseIgnoreError tries to close the MS window but won't
// return error if it fails. This is used mostly when closing window is the last
// step.
func (ms *Ms365) WaitForMicrosoft365WindowAndCloseIgnoreError(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return ms.waitForMicrosoft365WindowAndClose(tconn, fileName, true /*=ignoreCloseError*/)
}

// InstallPWA installs Office PWA.
func (ms *Ms365) InstallPWA(ctx context.Context, cr *chrome.Chrome, bt browser.Type) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	br, closeResource, _, err := browserfixt.SetUpBrowserOrUseCurrent(ctx, cr, bt)
	if err != nil {
		return errors.Wrap(err, "failed to get a browser to install PWA")
	}
	defer closeResource(cleanupCtx)

	// Use NewConn instead of NewTab to prevent Lacros from reusing the existing
	// chrome://newtab, the reuse will cause chrome://newtab to be closed below.
	conn, err := br.NewConn(ctx, officePWAInstallURL)
	if err != nil {
		return errors.Wrap(err, "failed to open office website")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

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

// MaybeUninstallPwa uninstalls the MS 365 PWA.
func MaybeUninstallPwa(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	appName := apps.Microsoft365.Name
	installed, err := ash.ChromeAppInstalled(ctx, tconn, apps.Microsoft365.ID)
	if err != nil {
		return errors.Wrap(err, "failed to check for Microsoft 365 PWA")
	}
	if !installed {
		return nil
	}

	testing.ContextLogf(ctx, "Uninstalling PWA: %s", appName)
	if err := ossettings.UninstallApp(ctx, tconn, cr, appName, apps.Microsoft365.ID); err != nil {
		return errors.Wrapf(err, "failed to uninstall the PWA %q", appName)
	}

	return nil
}

// ClearBrowserCookiesForOffice will clear all browser cookies for the Office website.
func ClearBrowserCookiesForOffice(ctx context.Context, cr *chrome.Chrome, bt browser.Type) error {
	testing.ContextLog(ctx, "Clearing cookies for Office website")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	br, closeResource, _, err := browserfixt.SetUpBrowserOrUseCurrent(ctx, cr, bt)
	if err != nil {
		return errors.Wrap(err, "failed to get a browser to clear cookies")
	}
	defer closeResource(cleanupCtx)

	interval := 200 * time.Millisecond

	return action.Retry(3, func(ctx context.Context) error {
		// Opening Office PWA is usually quick, but occasionally really slow,
		// with a shorter context it can try again if it hit a slow attempt.
		quickCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Use NewConn instead of NewTab to prevent Lacros from reusing the existing
		// chrome://newtab, the reuse will cause chrome://newtab to be closed below.
		conn, err := br.NewConn(quickCtx, officePWAInstallURL)
		if err != nil {
			return errors.Wrap(err, "failed to open office website")
		}
		defer conn.Close()
		defer conn.CloseTarget(ctx)

		return conn.ClearSiteCookies(quickCtx, "www.microsoft365.com")
	}, interval)(ctx)
}
