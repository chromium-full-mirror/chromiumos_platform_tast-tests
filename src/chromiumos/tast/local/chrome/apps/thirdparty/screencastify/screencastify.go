// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package screencastify

import (
	"chromiumos/tast/common/action"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/cws"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"

	"context"
	"regexp"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InstallExtension installs Screencastify Chrome extension from Chrome Web Store.
func InstallExtension(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser) error {
	return cws.InstallApp(ctx, br, tconn, cws.Screencastify)
}

// UninstallExtension uninstalls Screencastify Chrome extension from Chrome Web Store.
func UninstallExtension(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser) error {
	return cws.UninstallApp(ctx, br, tconn, cws.Screencastify)
}

var signInArea = nodewith.NameContaining("Google").Role(role.RootWebArea)
var signInWithGoogleButton = nodewith.NameContaining("Sign in with Google").Role(role.Button)

// Use First() to select the first account in the account list.
var accountSelectLink = nodewith.NameRegex(regexp.MustCompile("@.*.com")).Role(role.Link).Ancestor(signInArea).First()
var allowGoogleLoginButton = nodewith.Name("Allow").Role(role.Button).Ancestor(signInArea)

// Login authenticates in below steps.
// 1. Navigate to https://app.screencastify.com/extension-auth/handover to trigger login.
// 2. Login with Google account.
// 3. [Optional] Setup Screencastify for new user.
// 4. [Optional] Enable mic and camera.
func Login(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, br *browser.Browser) error {
	ui := uiauto.New(tconn)

	screencastifyRootWebView := nodewith.Name("Screencastify").Role(role.RootWebArea)
	screencastifyTermCheckbox := nodewith.NameContaining("I agree to Screencastify's Terms").Role(role.CheckBox).Ancestor(screencastifyRootWebView)
	screencastifyTermNextButton := nodewith.Name("Next").Role(role.Button).Ancestor(screencastifyRootWebView)
	screencastifyRegisterTextIdentifier := nodewith.NameContaining("Tell us a bit about yourself").Role(role.StaticText)
	// Select "Other" in the identity options.
	screencastifyIdentityOther := nodewith.NameContaining("🌎 Other").Role(role.Button)
	screencastifyPurposeOther := nodewith.NameContaining("📹 Other").Role(role.Button)
	screencastifyEnableMicAndWebCam := nodewith.Name("Enable mic and webcam").Role(role.Button)
	setupFinishFinder := nodewith.NameRegex(regexp.MustCompile("Welcome to Screencastify Record|Good to go")).Role(role.StaticText)

	authURL := "https://app.screencastify.com/extension-auth/handover"
	conn, err := br.NewTab(ctx, authURL)
	if err != nil {
		return errors.Wrapf(err, "failed to navigate to %s", authURL)
	}

	// signInWithGoogleButton: The login Google user has not yet granted permission to Screencastify.
	// screencastifyTermCheckbox: The login Google user has granted access already, but not yet setup Screencastify account.
	// screencastifyEnableMicAndWebCam: The login Google user already has Screencastify account.
	foundNode, err := ui.FindAnyExists(ctx, signInWithGoogleButton, screencastifyTermCheckbox, screencastifyEnableMicAndWebCam)
	if err != nil {
		// Sometimes the page is hanging on page loading. Refresh page to retry.
		if err := ui.WithTimeout(time.Minute).RetryUntil(
			br.ReloadActiveTab,
			func(ctx context.Context) error {
				foundNode, err = ui.FindAnyExists(ctx, signInWithGoogleButton, screencastifyTermCheckbox, screencastifyEnableMicAndWebCam)
				return err
			},
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to negavigate to Screencastify auth page")
		}
	}

	if foundNode == signInWithGoogleButton {
		if err := loginWithGoogleAccount(cr, tconn, conn)(ctx); err != nil {
			return errors.Wrap(err, "failed to sign in with Google account")
		}
	}

	// screencastifyTermCheckbox: This Google account has not setup Screencastify account.
	// screencastifyEnableMicAndWebCam: This device has not granted AV permissions to Screencastify.
	foundFinder, err := ui.FindAnyExists(ctx, screencastifyTermCheckbox, screencastifyEnableMicAndWebCam)
	if err != nil {
		return errors.Wrap(err, "failed to continue after Google account login")
	}

	if foundFinder == screencastifyTermCheckbox {
		if err := uiauto.NamedCombine("setup new account for Screencastify",
			ui.DoDefault(screencastifyTermCheckbox),
			ui.DoDefault(screencastifyTermNextButton),
			ui.WaitUntilExists(screencastifyRegisterTextIdentifier),
			ui.DoDefault(screencastifyIdentityOther),
			ui.DoDefault(screencastifyPurposeOther),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to setup account")
		}
	}

	if err := uiauto.NamedCombine("grant AV permissions",
		ui.DoDefault(screencastifyEnableMicAndWebCam),
		prompts.DismissPrompt(ui, prompts.AllowAVPermissionPrompt),
		ui.WaitUntilExists(setupFinishFinder),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to grant AV permissions")
	}

	return nil
}

func loginWithGoogleAccount(cr *chrome.Chrome, tconn *chrome.TestConn, conn *chrome.Conn) action.Action {
	ui := uiauto.New(tconn)
	return uiauto.NamedCombine("sign in with device account",
		ui.DoDefault(signInWithGoogleButton),
		func(ctx context.Context) error {
			if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
				return errors.Wrap(err, "failed to wait for page loading complete")
			}

			// Two situations need to handle here:
			// 1. Sometimes clicking account link does not work.
			//    The page should start to load if click works. Using this expectation to confirm.
			// 2. Login timeout and the page does not return. Should re-click the account to retry login.
			return testing.Poll(ctx, func(ctx context.Context) error {
				if err := ui.LeftClick(accountSelectLink)(ctx); err != nil {
					return errors.Wrap(err, "failed to click account")
				}
				if err := conn.WaitForExprWithTimeout(ctx, "document.readyState === 'loading'", 5*time.Second); err != nil {
					if accountLinkStillExist, err := ui.IsNodeFound(ctx, accountSelectLink); err != nil {
						return testing.PollBreak(errors.Wrap(err, "failed to check account link"))
					} else if !accountLinkStillExist {
						// Assume sometimes the login is super fast and bypass the page status transition.
						return nil
					}
					return errors.Wrap(err, "failed to wait for page starting to load")
				}
				// Login Google can take quite long sometimes, so using mediumUITimeout.
				return ui.WithTimeout(30 * time.Second).WaitUntilGone(accountSelectLink)(ctx)
			}, &testing.PollOptions{Timeout: time.Minute})
		},
		ui.DoDefault(allowGoogleLoginButton),
	)
}
