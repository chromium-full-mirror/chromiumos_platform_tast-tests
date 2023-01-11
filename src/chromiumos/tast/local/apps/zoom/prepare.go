// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/crostini/faillog"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

// navigateToZoomAndSignIn starts a new Chrome browser, navigates to the Zoom website and signs in if not yet.
func navigateToZoomAndSignIn(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (conn *chrome.Conn, cleanup action.Action, retErr error) {
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, zoomWebsite)
	if err != nil {
		return nil, nil, err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, err
	}

	defer func(ctx context.Context) {
		if retErr != nil {
			faillog.DumpUITreeAndScreenshot(ctx, tconn, "zoom_login", retErr)
			if err := closeBrowser(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close browser in cleanup")
			}
		}
	}(ctx)

	ui := uiauto.New(tconn)

	if err := acceptCookiePrompts(tconn)(ctx); err != nil {
		return nil, nil, errors.Wrap(retErr, "failed to dimiss cookie prompt")
	}

	// Sign in if needed.
	var nodeFound *nodewith.Finder
	nodeFound, err = ui.FindAnyExists(ctx, myAccountLink, myProfileImg, signInLink)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for either MY ACCOUNT or SIGN IN node")
	}

	if nodeFound == signInLink {
		testing.ContextLog(ctx, "Sign in Zoom")
		if err := signIn(ctx, conn, tconn); err != nil {
			return nil, nil, errors.Wrap(err, "failed to sign-in")
		}
	}

	// Register new account if required.
	nodeFound, err = ui.FindAnyExists(ctx, myAccountLink, myProfileImg, agreeToTermsArea)
	if retErr != nil {
		return nil, nil, errors.Wrap(err, "failed to reach either my account or registration flow")
	}

	if nodeFound == agreeToTermsArea {
		testing.ContextLog(ctx, "Creating new Zoom account")
		if err := createAccount(ctx, tconn); err != nil {
			return nil, nil, errors.Wrap(err, "failed to create account")
		}
	}

	return conn, closeBrowser, nil
}

func signIn(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn) error {
	if err := conn.Navigate(ctx, "https://zoom.us/google_oauth_signin"); err != nil {
		return err
	}

	ui := uiauto.New(tconn)

	signInArea := nodewith.Name("Sign in – Google accounts").Role(role.RootWebArea)
	accountSelectLink := nodewith.NameRegex(regexp.MustCompile("@gmail.com")).Role(role.Link).Ancestor(signInArea)
	return ui.LeftClickUntil(accountSelectLink,
		ui.WithTimeout(shortUITimeout).WaitUntilGone(accountSelectLink))(ctx)
}

func createAccount(ctx context.Context, tconn *chrome.TestConn) error {
	// Creating new account starts with age verification.
	verifyYourAge := nodewith.NameContaining("Verify Your Age").Role(role.Heading).Ancestor(agreeToTermsArea)
	continueButton := nodewith.Name("Continue").Role(role.Button).Ancestor(agreeToTermsArea)
	createAccountButton := nodewith.Name("Create Account").Role(role.Button).Ancestor(agreeToTermsArea)

	ui := uiauto.New(tconn)

	if err := ui.WithTimeout(shortUITimeout).WaitUntilExists(verifyYourAge)(ctx); err != nil {
		return err
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}

	inputField := nodewith.Role(role.TextField).Ancestor(agreeToTermsArea)
	if err := uiauto.Combine("input age to verify",
		ui.FocusAndWait(inputField),
		kb.TypeAction("2000"), // Hardcode Year-of-birth to 2000.
		ui.DoDefaultUntil(continueButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(continueButton)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify age")
	}

	if err := ui.DoDefaultUntil(createAccountButton,
		ui.WithTimeout(shortUITimeout).WaitUntilGone(createAccountButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to create account")
	}
	return nil
}

// launchNewMeeting creates a new meeting and choose to join meeting via browser.
func launchNewMeeting(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	if err := conn.Navigate(ctx, startNewMeetingURL); err != nil {
		return err
	}

	// After navigating to the start new meeting url. 3 possible results can be expected sequentially:
	// 1. (Optional) The user is already in another meeting. It asks confirmation to start this one.
	// 2. (Optional) It is the only meeting of this user. It continues to ask how to join the meeting.
	// 3. The user enters the meeting page.
	joinFromYourBrowser := nodewith.Name("Join from Your Browser").Role(role.StaticText)
	// If user is already in another meeting, then we need to explicitly end the meeting before joining this one.
	startThisMeetingButton := nodewith.Name("Start this Meeting").Role(role.Button).Ancestor(zoomMainWebArea)

	if foundNode, err := ui.FindAnyExists(ctx, startThisMeetingButton, joinFromYourBrowser, mainLayoutCanvas); err != nil {
		return errors.Wrap(err, "failed to create new meeting")
	} else if foundNode == startThisMeetingButton {
		// Stop previous meeting takes a bit time, so using longer wait here.
		if err := ui.WithTimeout(longUITimeout).DoDefaultUntil(startThisMeetingButton,
			ui.WaitUntilGone(startThisMeetingButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to stop previous meeting")
		}
	}

	if foundNode, err := ui.FindAnyExists(ctx, joinFromYourBrowser, mainLayoutCanvas); err != nil {
		return errors.Wrap(err, "failed to join new meeting")
	} else if foundNode == joinFromYourBrowser {
		if err := ui.DoDefaultUntil(joinFromYourBrowser,
			ui.WithTimeout(5*time.Second).WaitUntilGone(joinFromYourBrowser),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to join meeting from browser")
		}
	}
	return ui.WaitUntilExists(mainLayoutCanvas)(ctx)
}

func acceptCookiePrompts(tconn *chrome.TestConn) action.Action {
	// There are two types of cookie accept dialogs: "ACCEPT COOKIES" and "ACCEPT ALL COOKIES".
	acceptCookieButton := nodewith.NameRegex(regexp.MustCompile("ACCEPT.*COOKIES")).Role(role.Button)
	acceptCookiesPrompt := prompts.Prompt{
		Name:              "Accept cookies",
		PromptFinder:      acceptCookieButton,
		ClearButtonFinder: acceptCookieButton,
	}

	return prompts.ClearPotentialPrompts(tconn, shortUITimeout, acceptCookiesPrompt)
}
