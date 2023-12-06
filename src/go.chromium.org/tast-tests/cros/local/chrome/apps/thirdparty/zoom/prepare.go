// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// navigateToZoomAndSignIn starts a new Chrome browser, navigates to the Zoom website and signs in if not yet.
func navigateToZoomAndSignIn(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn) error {
	if err := conn.Navigate(ctx, zoomWebsite); err != nil {
		return err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	ui := uiauto.New(tconn)

	if err := acceptCookiePrompts(tconn)(ctx); err != nil {
		return errors.Wrap(err, "failed to dimiss cookie prompt")
	}

	// Sign in if needed.
	var nodeFound *nodewith.Finder
	nodeFound, err = ui.FindAnyExists(ctx, myAccountLink, myProfileImg, signInLink)
	if err != nil {
		return errors.Wrap(err, "failed to wait for either MY ACCOUNT or SIGN IN node")
	}

	if nodeFound == signInLink {
		testing.ContextLog(ctx, "Sign in Zoom")
		if err := signIn(ctx, conn, tconn); err != nil {
			return errors.Wrap(err, "failed to sign-in")
		}
	}

	// Register new account if required.
	// Sometimes it takes a long time to display the next page.
	continueButton := nodewith.Name("Continue").Role(role.Button)
	nodeFound, err = ui.WithTimeout(3*time.Minute).FindAnyExists(ctx, myAccountLink, myProfileImg, agreeToTermsArea, continueButton)
	if err != nil {
		return errors.Wrap(err, "failed to reach either my account or registration flow")
	}
	if nodeFound == continueButton {
		if err := ui.LeftClick(continueButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click continue button")
		}
	}
	if nodeFound == agreeToTermsArea {
		testing.ContextLog(ctx, "Creating new Zoom account")
		if err := createAccount(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create account")
		}
	}

	return nil
}

func signIn(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn) error {
	if err := conn.Navigate(ctx, "https://zoom.us/google_oauth_signin"); err != nil {
		return err
	}

	ui := uiauto.New(tconn)

	signInArea := nodewith.NameContaining("Google").Role(role.RootWebArea)
	// Use First() to select the first account in the account list.
	accountSelectLink := nodewith.NameRegex(regexp.MustCompile("@.*.com")).Role(role.Link).Ancestor(signInArea).First()

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
		if err := conn.WaitForExprWithTimeout(ctx, "document.readyState === 'loading'", 10*time.Second); err != nil {
			if accountLinkStillExist, err := ui.IsNodeFound(ctx, accountSelectLink); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to check account link"))
			} else if !accountLinkStillExist {
				// Assume sometimes the login is super fast and bypass the page status transition.
				return nil
			}
			return errors.Wrap(err, "failed to wait for page starting to load")
		}
		// Login Google can take quite long sometimes, so using mediumUITimeout.
		return ui.WithTimeout(mediumUITimeout).WaitUntilGone(accountSelectLink)(ctx)
	}, &testing.PollOptions{Timeout: 2 * time.Minute})
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
		ui.DoDefaultUntil(inputField, ui.WithTimeout(shortUITimeout).WaitUntilExists(inputField.Focused())),
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
func launchNewMeeting(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, meetingURL string) error {
	// Launch meeting sometimes hangs on loading meeting page. e.g. http://screen/KWBhnx6UUjtbxnL
	// Considerding Zoom is an external app and potentially flaky adding this retry should mitigate the problem.
	return uiauto.Retry(3, func(ctx context.Context) error {
		if err := conn.Navigate(ctx, meetingURL); err != nil {
			return err
		}
		return enterNewMeeting(ctx, tconn)
	})(ctx)
}

func enterNewMeeting(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	// After navigating to the start new meeting url. 4 possible results can be expected sequentially:
	// 1. (Optional) The user is already in another meeting. It asks confirmation to start this one.
	// 2. (Optional) It is the only meeting of this user. It continues to ask how to join the meeting.
	// 3. (Optional) If meetingURL is invite link, there will be the join page to enter meeting page.
	// 4. The user enters the meeting page.
	joinFromYourBrowser := nodewith.Name("Join from Your Browser").Role(role.StaticText)
	// If user is already in another meeting, then we need to explicitly end the meeting before joining this one.
	startThisMeetingButton := nodewith.Name("Start this Meeting").Role(role.Button).Ancestor(zoomMainWebArea)
	joinButton := nodewith.Name("Join").Role(role.Button).Ancestor(zoomMainWebArea)
	notificationsPrompt := prompts.ShowNotificationsPrompt.PromptFinder
	avPermissionPrompt := prompts.AllowAVPermissionPrompt.PromptFinder
	noPermissionText := nodewith.Name("No permission. (200)").Role(role.StaticText)
	foundNode, err := ui.WithTimeout(longUITimeout).FindAnyExists(ctx,
		startThisMeetingButton,
		joinFromYourBrowser,
		mainLayoutCanvas,
		endMenu,
		leaveButton,
		notificationsPrompt,
		avPermissionPrompt,
		noPermissionText)
	if err != nil {
		return errors.Wrap(err, "failed to enter new meeting")
	}
	switch foundNode {
	case startThisMeetingButton:
		// Stop previous meeting takes a bit time, so using longer wait here.
		if err := ui.WithTimeout(longUITimeout).DoDefaultUntil(startThisMeetingButton,
			ui.WithTimeout(mediumUITimeout).WaitUntilGone(startThisMeetingButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to stop previous meeting")
		}
	case joinFromYourBrowser:
		if err := ui.DoDefaultUntil(joinFromYourBrowser,
			ui.WithTimeout(5*time.Second).WaitUntilGone(joinFromYourBrowser),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to join meeting from browser")
		}
	case joinButton:
		previewVideo := nodewith.ClassName("preview-video").Ancestor(zoomMainWebArea)
		// In Zoom website, the join button may be hidden in tablet mode.
		// Make it visible before clicking.
		// Since ui.MakeVisible() is not always successful, add a retry here.
		clickJoinButton := ui.Retry(3, uiauto.Combine("click join button",
			ui.WaitForLocation(joinButton),
			ui.MakeVisible(joinButton),
			ui.LeftClickUntil(joinButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(joinButton)),
		))

		if err := uiauto.NamedCombine("join meeting",
			ui.WaitUntilExists(previewVideo),
			allowPerm(tconn),
			clickJoinButton,
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to join meeting")
		}
	case notificationsPrompt, avPermissionPrompt:
		if err := prompts.ClearPotentialPrompts(
			tconn,
			shortUITimeout,
			prompts.ShowNotificationsPrompt,
			prompts.AllowAVPermissionPrompt,
		)(ctx); err != nil {
			return err
		}
	case noPermissionText:
		return errors.Wrap(err, `the "No Permission" problem is displayed, zoom account may require re-registration`)
	case mainLayoutCanvas, endMenu, leaveButton:
		// Joining meeting can take a while, it sometimes hangs on `Joining Meeting...` screen.
		joiningMeetingText := nodewith.Name("Joining Meeting...").Role(role.StaticText).First()
		return uiauto.IfSuccessThen(ui.WithTimeout(5*time.Second).WaitUntilExists(joiningMeetingText),
			ui.WithTimeout(mediumUITimeout).WaitUntilGone(joiningMeetingText))(ctx)
	default:
		return errors.New("failed to enter new meeting")
	}

	return enterNewMeeting(ctx, tconn)
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

// allowPerm allows camera and microphone if browser asks for the permissions.
func allowPerm(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	joinAudioButton := nodewith.Name("Join Audio").Role(role.Button)
	spinnerImage := nodewith.ClassName("spinner").Role(role.Image).First()
	checkSpinnerImageExists := ui.WithTimeout(shortUITimeout).WaitUntilExists(spinnerImage)
	allowPermissions := prompts.ClearPotentialPrompts(tconn, shortUITimeout, prompts.ShowNotificationsPrompt, prompts.AllowAVPermissionPrompt)
	return uiauto.Combine("allow permissions",
		uiauto.IfSuccessThen(ui.Exists(startVideoButton),
			ui.LeftClickUntil(startVideoButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(startVideoButton))),
		uiauto.IfSuccessThen(ui.Exists(joinAudioButton),
			ui.LeftClickUntil(joinAudioButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(joinAudioButton))),
		// Check the spinner image to quickly determine if it requires permission to allow microphone and camera.
		uiauto.IfSuccessThen(checkSpinnerImageExists, allowPermissions),
		apps.AllowPagePermissions(tconn),
	)
}

// GrantPermissions grants Microphone, Camera and Notifications permissions to Zoom.
func GrantPermissions(ctx context.Context, br *browser.Browser) error {
	zoomURLPatterns := []string{"*://zoom.us/*", "*://*.zoom.us/*"}

	return br.GrantPermissions(ctx, zoomURLPatterns,
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
		browser.NotificationsContentSetting,
	)
}
