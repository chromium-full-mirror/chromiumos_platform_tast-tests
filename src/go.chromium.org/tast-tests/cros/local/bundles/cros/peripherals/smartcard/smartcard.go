// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package smartcard provides utility functions for running smartcard tast tests.
package smartcard

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SmartCard holds the resources required for running smart card tests.
type SmartCard struct {
	cr            *chrome.Chrome
	kb            *input.KeyboardEventWriter
	ui            *uiauto.Context
	adminUsername string
	adminPassword string
}

const shortTimeout = 5 * time.Second

var (
	signInUsingACertificateButton = nodewith.Name("Sign in using a certificate").Role(role.Button)
	enterYourPINText              = nodewith.Name("Enter your PIN").Role(role.StaticText)
	smartCardPINDialog            = nodewith.Name("Smart card PIN").Role(role.Dialog).First()
	emailTextField                = nodewith.Name("Email or phone").Role(role.TextField)
	okButton                      = nodewith.Name("OK").Role(role.Button)
)

// New creates a new SmartCard instance.
func New(cr *chrome.Chrome, tLoginConn *chrome.TestConn, kb *input.KeyboardEventWriter, adminUsername, adminPassword string) *SmartCard {
	ui := uiauto.New(tLoginConn)
	return &SmartCard{
		cr:            cr,
		kb:            kb,
		ui:            ui,
		adminUsername: adminUsername,
		adminPassword: adminPassword,
	}
}

// EnterPINPage returns an action that enters PIN page by clicking
// on "Sign in using a certificate" button.
func (s *SmartCard) EnterPINPage() action.Action {
	return uiauto.NamedCombine("enter PIN page",
		s.ui.DoDefault(signInUsingACertificateButton),
		s.ui.WaitUntilExists(enterYourPINText),
	)
}

// SetPIN returns an action that sets PIN code on PIN page.
func (s *SmartCard) SetPIN(pinCode string) action.Action {
	return uiauto.NamedCombine(fmt.Sprintf("set pin to %q", pinCode),
		s.ui.WaitUntilAnyExists(enterYourPINText, smartCardPINDialog),
		s.kb.TypeAction(pinCode),
		s.kb.AccelAction("Enter"),
	)
}

// SignOut returns an action that signs out smart card.
func (s *SmartCard) SignOut() action.Action {
	signOutButton := nodewith.Name("Sign out").Role(role.Button).HasClass("MdTextButton")
	signOutNowButton := nodewith.Name("Sign out now").Role(role.Button)
	return uiauto.NamedCombine("sign out",
		s.ui.DoDefault(signOutButton),
		s.ui.WaitUntilExists(signOutNowButton),
		s.kb.AccelAction("Enter"),
	)
}

// SignIn returns an action that signs in smart card.
func (s *SmartCard) SignIn() action.Action {
	signInButton := nodewith.Name("Sign in with smart card").Role(role.Button)
	unrecognizedText := nodewith.Name("Couldn’t recognize your smart card. Try again.").Role(role.StaticText)
	return uiauto.NamedCombine("sign in",
		s.ui.LeftClickUntil(signInButton,
			s.ui.WithTimeout(time.Second).WaitUntilAnyExists(smartCardPINDialog, unrecognizedText)),
	)
}

// AddPerson returns an action that adds person and enters PIN page.
func (s *SmartCard) AddPerson() action.Action {
	addPersonButton := nodewith.Name("Add Person").Role(role.Button)
	return uiauto.NamedCombine("add person",
		s.ui.DoDefault(addPersonButton),
		s.EnterPINPage(),
	)
}

// SecurityPolicyOption represents the different security options that can be applied.
type SecurityPolicyOption string

const (
	// SecurityPolicyLogOut indicates that the user should be logged out.
	SecurityPolicyLogOut SecurityPolicyOption = "Log the user out"
	// SecurityPolicyLock indicates that the current user session should be locked.
	SecurityPolicyLock SecurityPolicyOption = "Lock the current session"
)

// SetSecurityTokenRemoval returns an action that sets security policy
// and removal notification duration.
func (s *SmartCard) SetSecurityTokenRemoval(policy SecurityPolicyOption, duration string) action.Action {
	const adminURL = "https://admin.google.com"
	ui := s.ui
	kb := s.kb

	passwordField := nodewith.Name("Enter your password").Role(role.TextField)
	logInAsAdministratorUser := uiauto.NamedCombine("log in as administrator user",
		ui.LeftClickUntil(emailTextField, ui.WithTimeout(shortTimeout).WaitUntilExists(emailTextField.Focused())),
		kb.TypeAction(s.adminUsername),
		kb.AccelAction("Enter"),
		ui.DoDefault(passwordField),
		kb.TypeAction(s.adminPassword),
		kb.AccelAction("Enter"),
	)

	mainMenuButton := nodewith.Name("Main menu").Role(role.Button).First()
	chromeBrowserInlineTextBox := nodewith.Name("Chrome browser").Role(role.InlineTextBox).First()
	settingsInlineTextBox := nodewith.Name("Settings").Role(role.InlineTextBox).First()
	searchTextField := nodewith.Name("Search for organizational units").Role(role.TextField)
	goToBrowserSettings := uiauto.NamedCombine("go to 'Chrome Browser > Settings > User & Browser settings'",
		ui.DoDefaultUntil(mainMenuButton, ui.WithTimeout(shortTimeout).WaitUntilExists(chromeBrowserInlineTextBox)),
		ui.DoDefaultUntil(chromeBrowserInlineTextBox, ui.WithTimeout(shortTimeout).WaitUntilExists(settingsInlineTextBox)),
		ui.DoDefaultUntil(settingsInlineTextBox, ui.WithTimeout(shortTimeout).WaitUntilExists(searchTextField)),
	)

	smartcardGroup := nodewith.Name("smartcard").Role(role.Group)
	selectSmartCardGroup := uiauto.NamedCombine("select smart card group",
		ui.WaitUntilExists(searchTextField),
		ui.LeftClickUntil(searchTextField, ui.WithTimeout(shortTimeout).WaitUntilExists(searchTextField.Focused())),
		kb.TypeAction("smartcard"),
		ui.DoDefault(smartcardGroup),
	)

	searchTextFieldWithComboBox := nodewith.NameContaining("Search or add a filter").Role(role.TextFieldWithComboBox)
	securityTokenRemovalLink := nodewith.Name("Security token removal").Role(role.Link)
	goToSecurityTokenRemoval := uiauto.NamedCombine("go to 'Security token removal' setting",
		ui.LeftClickUntil(searchTextFieldWithComboBox, ui.WithTimeout(shortTimeout).WaitUntilExists(searchTextFieldWithComboBox.Focused())),
		kb.TypeAction("security"),
		kb.AccelAction("Enter"),
		ui.DoDefault(securityTokenRemovalLink),
	)

	selected := func(finder *nodewith.Finder) uiauto.Action {
		return func(ctx context.Context) error {
			nodeInfo, err := s.ui.Info(ctx, finder)
			if err != nil {
				return err
			}
			if !nodeInfo.Selected {
				return errors.Wrapf(err, "%q selected state is not true", nodeInfo.Name)
			}
			return nil
		}
	}

	policyOption := nodewith.Name(string(policy)).Role(role.ListBoxOption)
	selectPolicyOption := uiauto.NamedCombine(fmt.Sprintf("select policy option to %q", policy),
		ui.WaitUntilExists(policyOption),
		ui.MakeVisible(policyOption),
		uiauto.IfFailThen(selected(policyOption),
			ui.RetryUntil(ui.DoDefault(policyOption), selected(policyOption))),
	)

	durationTextField := nodewith.Name("Removal notification duration (seconds)").Role(role.TextField)
	durationText := nodewith.Name(duration).Role(role.StaticText).Ancestor(durationTextField)
	setDuration := uiauto.IfSuccessThen(ui.Gone(durationText),
		uiauto.NamedCombine(fmt.Sprintf("set duration to %q", duration),
			ui.MakeVisible(durationTextField),
			ui.LeftClickUntil(durationTextField, ui.WithTimeout(shortTimeout).WaitUntilExists(durationTextField.Focused())),
			kb.AccelAction("Ctrl+A"),
			kb.TypeAction(duration),
			ui.WaitUntilExists(durationText),
		))

	saveButton := nodewith.Name("Save").Role(role.Button)
	saveSetting := uiauto.NamedCombine("save setting",
		ui.DoDefault(saveButton),
		ui.WaitUntilGone(saveButton.Focusable()),
	)

	return uiauto.NamedCombine("set security token removal",
		s.openIncognitoMaximizedWindow(adminURL),
		logInAsAdministratorUser,
		goToBrowserSettings,
		selectSmartCardGroup,
		goToSecurityTokenRemoval,
		selectPolicyOption,
		setDuration,
		saveSetting,
	)
}

// WaitForAutoLockOrLogOut returns an action that waits for auto lock or log out.
func (s *SmartCard) WaitForAutoLockOrLogOut(policy SecurityPolicyOption, duration time.Duration) action.Action {
	// If duration is set to n seconds, the countdown text will start to display from n-1 seconds.
	countdownSeconds := (duration.Seconds() - 1)
	notification := fmt.Sprintf("Your Chromebook will be locked automatically in %v seconds", countdownSeconds)
	if policy == SecurityPolicyLogOut {
		notification = fmt.Sprintf("You will be automatically signed out in %v seconds", countdownSeconds)
	}
	notificationText := nodewith.NameContaining(notification).Role(role.StaticText)
	return uiauto.NamedCombine(fmt.Sprintf("wait for auto %q", policy),
		uiauto.NamedAction(fmt.Sprintf("wait for the notification %q", notification),
			s.ui.WithTimeout(30*time.Second).WaitUntilExists(notificationText),
		),
		uiauto.NamedAction(fmt.Sprintf("wait for %v", duration), uiauto.Sleep(duration)),
	)
}

// WebApplicationAuthentication performs authentication in a web app using a smart card.
func (s *SmartCard) WebApplicationAuthentication(account, pinCode string) action.Action {
	const mailURL = "https://mail.google.com"
	ui := s.ui
	kb := s.kb

	passphraseTextfield := nodewith.Role(role.TextField).HasClass("PassphraseTextfield")
	mailHeading := nodewith.Name("Mail").Role(role.Heading)
	return uiauto.NamedCombine("web application authentication with smart card",
		s.openIncognitoMaximizedWindow(mailURL),
		ui.LeftClickUntil(emailTextField, ui.WithTimeout(shortTimeout).WaitUntilExists(emailTextField.Focused())),
		kb.TypeAction(account),
		kb.AccelAction("Enter"),
		ui.DoDefault(signInUsingACertificateButton),
		ui.DoDefaultUntil(okButton,
			ui.WithTimeout(shortTimeout).WaitUntilExists(passphraseTextfield)),
		kb.TypeAction(pinCode),
		ui.DoDefault(okButton),
		ui.WaitUntilExists(mailHeading),
	)
}

// DriveLockCSSIAppTesting launches the DriveLock CSSI app, enters the PIN, and
// verifies the signing process.
func (s *SmartCard) DriveLockCSSIAppTesting(ctx context.Context, tconn *chrome.TestConn, pinCode string) error {
	appID := apps.DriveLockSmartCardMiddleware.ID
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := apps.Launch(ctx, tconn, appID); err != nil {
		return errors.Wrap(err, "failed to launch DriveLock CSSI app")
	}
	defer apps.Close(cleanupCtx, tconn, appID)

	testNowButton := nodewith.Name("Test now").Role(role.Button)
	textField := nodewith.Name("Please enter your Smart Card PIN:").Role(role.TextField).Focused()
	okText := nodewith.Name("Signing: OK").Role(role.InlineTextBox)
	return uiauto.NamedCombine("test drive lock CSSI app",
		s.ui.DoDefault(testNowButton),
		s.ui.WaitUntilExists(textField),
		s.kb.TypeAction(pinCode),
		s.ui.DoDefault(okButton),
		s.ui.WaitUntilExists(okText),
	)(ctx)
}

// VerifyCertificate opens the certificates settings page and verifies the
// account's certificate.
func (s *SmartCard) VerifyCertificate(ctx context.Context, cr *chrome.Chrome, account string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testing.ContextLog(ctx, "Opening the certificates tab")
	conn, err := cr.NewConn(ctx, "chrome://settings/certificates")
	if err != nil {
		return errors.Wrap(err, "failed to open the certificates tab")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	certificatesButton := nodewith.Name("Show certificates for organization").Role(role.Button)
	accountText := nodewith.Name(fmt.Sprintf("%s (extension provided)", account)).Role(role.StaticText)
	return uiauto.NamedCombine(fmt.Sprintf("verify certificate: %v", account),
		s.ui.DoDefault(certificatesButton),
		s.ui.WaitUntilExists(accountText),
	)(ctx)
}

// UpdateInstance sets the SmartCard instance with new Chrome and UI automation connections.
func (s *SmartCard) UpdateInstance(cr *chrome.Chrome, tLoginConn *chrome.TestConn) {
	s.cr = cr
	s.ui = uiauto.New(tLoginConn)
}

func (s *SmartCard) openIncognitoMaximizedWindow(url string) action.Action {
	return func(ctx context.Context) error {
		// Launch incognito Chrome browser by pressing keyboard "Ctrl+Shift+N".
		if err := s.kb.Accel(ctx, "Ctrl+Shift+N"); err != nil {
			return errors.Wrap(err, "failed to launch incognito Chrome browser")
		}
		conn, err := s.cr.NewConnForTarget(ctx, chrome.MatchTargetURL(chrome.NewTabURL))
		if err != nil {
			return errors.Wrap(err, "failed getting connection to new target")
		}
		defer conn.Close()
		if err = conn.Navigate(ctx, url); err != nil {
			return errors.Wrapf(err, "failed to navigate to %q", url)
		}
		tconn, err := s.cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get ash tconn")
		}
		w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch())
		if err != nil {
			return errors.Wrap(err, "failed to open a browser window")
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize the browser window")
		}
		return nil
	}
}
