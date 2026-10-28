// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ms365 contains the page object to interact with Microsoft login
// page and the Microsoft 365 app window.
package ms365

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
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
	defaultUITimeout    = 15 * time.Second
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

// ChooseSearchEngine proceeds through "ChooseSearchEngine" popup that is opened on a first run in some regions.
func (ms *Ms365) ChooseSearchEngine() uiauto.Action {
	chooseSearchEngine := nodewith.Role(role.Window).ClassName("SearchEngineChoiceDialogView")
	firstSearchEngine := nodewith.Ancestor(chooseSearchEngine).Role(role.RadioButton).First()
	firstSearchEngineOnscreen := nodewith.Ancestor(firstSearchEngine).Onscreen().First()
	closeButton := nodewith.Ancestor(chooseSearchEngine).Role(role.Button).ClassName("action-button")

	return func(ctx context.Context) error {
		if err := ms.ui.EnsureGoneFor(chooseSearchEngine, 5*time.Second)(ctx); err != nil {
			return uiauto.Combine("Choosing search engine",
				ms.ui.DoDefaultUntil(firstSearchEngine, ms.ui.Exists(firstSearchEngineOnscreen)),
				ms.ui.DoDefaultUntil(closeButton, ms.ui.Gone(chooseSearchEngine)),
			)(ctx)
		}
		return nil
	}
}

// InputUserName waits for the Microsoft sign in window and inputs or selects the username.
func (ms *Ms365) InputUserName(userName string) uiauto.Action {
	return ms.inputUserName(userName, nil)
}

func (ms *Ms365) inputUserName(userName string, abortFinder *nodewith.Finder) uiauto.Action {
	msSignInWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Pick an account)")).First()
	usernameInput := nodewith.Ancestor(msSignInWindow).Role(role.TextField).NameRegex(regexp.MustCompile("(?i).*(email|someone@example.com).*"))
	visibleUsernameInput := usernameInput.Visible()
	accountTile := nodewith.Ancestor(msSignInWindow).Role(role.Button).NameContaining(userName).First()
	useAnotherAccountButton := nodewith.Ancestor(msSignInWindow).Role(role.Button).Name("Use another account").First()

	enterUsername := uiauto.Combine("Enter MS username",
		ms.ui.WaitUntilExists(visibleUsernameInput),
		ms.ui.LeftClickUntilFocused(usernameInput),
		ms.kb.TypeAction(userName),
		ms.kb.AccelAction("Enter"),
		ms.ui.WaitUntilGone(visibleUsernameInput),
	)

	finders := []*nodewith.Finder{
		visibleUsernameInput,
		accountTile,
		useAnotherAccountButton,
	}
	if abortFinder != nil {
		finders = append(finders, abortFinder)
	}

	return func(ctx context.Context) error {
		if abortFinder != nil {
			foundWin, err := ms.ui.FindAnyExists(ctx, msSignInWindow, abortFinder)
			if err != nil {
				return errors.Wrap(err, "failed waiting for Microsoft sign-in window")
			}
			if foundWin == abortFinder {
				return errOneDriveRetryNeeded
			}
		} else if err := ms.ui.WaitUntilExists(msSignInWindow)(ctx); err != nil {
			return err
		}
		if err := ms.ChooseSearchEngine()(ctx); err != nil {
			return err
		}

		found, err := ms.ui.FindAnyExists(ctx, finders...)
		if err != nil {
			return errors.Wrap(err, "failed to find Microsoft username input or account chooser")
		}
		if abortFinder != nil && found == abortFinder {
			return errOneDriveRetryNeeded
		}

		// When "Pick an account" renders, both accountTile (if userName is signed in)
		// and useAnotherAccountButton exist simultaneously. Always prefer accountTile
		// if the target userName is present in the chooser.
		if found == accountTile || (found == useAnotherAccountButton && ms.ui.Exists(accountTile)(ctx) == nil) {
			testing.ContextLogf(ctx, "Selecting existing Microsoft account tile for %s", userName)
			return ms.ui.LeftClickUntil(accountTile, ms.ui.Gone(accountTile))(ctx)
		}
		if found == useAnotherAccountButton {
			testing.ContextLog(ctx, "Clicking 'Use another account' on Microsoft account chooser")
			if err := ms.ui.LeftClickUntil(useAnotherAccountButton, ms.ui.Exists(visibleUsernameInput))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Use another account'")
			}
		}
		return enterUsername(ctx)
	}
}

// UsePasswordInsteadOfCode proceeds when Microsoft tries to send a code during login.
func (ms *Ms365) UsePasswordInsteadOfCode(msSignInWindow *nodewith.Finder) uiauto.Action {
	otherWaysButton := nodewith.Ancestor(msSignInWindow).Role(role.Button).NameRegex(regexp.MustCompile("(?i)Other ways to sign in.*")).First()
	useYourPasswordInsteadButton := nodewith.Ancestor(msSignInWindow).NameRegex(regexp.MustCompile("(?i).*Use your password.*")).First()
	passwordInput := nodewith.Ancestor(msSignInWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(p|P)assword.*"))
	return func(ctx context.Context) error {
		found, err := ms.ui.FindAnyExists(ctx, passwordInput.Visible(), otherWaysButton, useYourPasswordInsteadButton)
		if err != nil {
			return errors.Wrap(err, "failed to find password input or password sign-in option")
		}
		if found == otherWaysButton {
			if err := ms.ui.LeftClickUntil(otherWaysButton, ms.ui.Exists(useYourPasswordInsteadButton))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Other ways to sign in'")
			}
			found = useYourPasswordInsteadButton
		}
		if found == useYourPasswordInsteadButton {
			if err := ms.ui.LeftClickUntil(useYourPasswordInsteadButton, ms.ui.Exists(passwordInput.Visible()))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Use your password'")
			}
		}
		return nil
	}
}

var errOneDriveRetryNeeded = errors.New("OneDrive connection failed after Microsoft interstitial; retry needed")

// inputUserNameOnRetry handles the Microsoft sign-in screen when retrying after an aborted OAuth
// popup, supporting standard email entry, "Pick an account", or warm-cookie SSO skipping straight
// to password or post-password screens.
func (ms *Ms365) inputUserNameOnRetry(userName string, setupCompleteDialogFinder, abortFinder *nodewith.Finder) uiauto.Action {
	msSignInWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Pick an account)")).First()
	usernameInput := nodewith.Ancestor(msSignInWindow).Role(role.TextField).NameRegex(regexp.MustCompile("(?i).*(email|someone@example.com).*"))
	visibleUsernameInput := usernameInput.Visible()
	accountTile := nodewith.Ancestor(msSignInWindow).Role(role.Button).NameContaining(userName).First()
	useAnotherAccountButton := nodewith.Ancestor(msSignInWindow).Role(role.Button).Name("Use another account").First()

	msPasswordWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Enter your password)|(Get a code to sign in)|(Other ways to sign in)|(Choose a way to sign in)|(Verify your identity)")).First()
	passwordInput := nodewith.Ancestor(msPasswordWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(p|P)assword.*"))
	visiblePasswordInput := passwordInput.Visible()
	otherWaysButton := nodewith.Ancestor(msPasswordWindow).Role(role.Button).NameRegex(regexp.MustCompile("(?i)Other ways to sign in.*")).First()
	useYourPasswordInsteadButton := nodewith.Ancestor(msPasswordWindow).NameRegex(regexp.MustCompile("(?i).*Use your password.*")).First()

	msStaySignedInWindowName := regexp.MustCompile("(Microsoft account|Sign in to your( Microsoft)? account|" + regexp.QuoteMeta("Stay signed in?") + ")")
	msStaySignedInWindow := nodewith.Role(role.RootWebArea).NameRegex(msStaySignedInWindowName)
	msStaySignedInRoot := nodewith.Role(role.RootWebArea).Name("Stay signed in?").First()
	msStaySignedInHeading := nodewith.Ancestor(msStaySignedInWindow).Role(role.Heading).Name("Stay signed in?").First()
	msUpdateTermsWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i).*updating our terms.*")).First()
	msConfirmSignInWindow := nodewith.Role(role.RootWebArea).Name("Is your security info still accurate?")
	msAcceptPermissionWindow := nodewith.Role(role.RootWebArea).NameContaining("Let this app access your info?")

	if setupCompleteDialogFinder == nil {
		setupCompleteDialogFinder = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Microsoft OneDrive connected|Microsoft 365 setup complete)"))
	}

	finders := []*nodewith.Finder{
		visibleUsernameInput,
		accountTile,
		useAnotherAccountButton,
		visiblePasswordInput,
		otherWaysButton,
		useYourPasswordInsteadButton,
		msUpdateTermsWindow,
		msConfirmSignInWindow,
		msStaySignedInRoot,
		msStaySignedInHeading,
		msAcceptPermissionWindow,
		setupCompleteDialogFinder,
	}
	if abortFinder != nil {
		finders = append(finders, abortFinder)
	}

	return func(ctx context.Context) error {
		found, err := ms.ui.FindAnyExists(ctx, finders...)
		if err != nil {
			return errors.Wrap(err, "failed to find Microsoft sign-in prompt on retry")
		}
		if abortFinder != nil && found == abortFinder {
			return errOneDriveRetryNeeded
		}
		if found == accountTile || (found == useAnotherAccountButton && ms.ui.Exists(accountTile)(ctx) == nil) {
			testing.ContextLogf(ctx, "Selecting existing Microsoft account tile for %s on retry", userName)
			return ms.ui.LeftClickUntil(accountTile, ms.ui.Gone(accountTile))(ctx)
		}
		if found == useAnotherAccountButton {
			testing.ContextLog(ctx, "Clicking 'Use another account' on Microsoft account chooser on retry")
			if err := ms.ui.LeftClickUntil(useAnotherAccountButton, ms.ui.Exists(visibleUsernameInput))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Use another account' on retry")
			}
			found = visibleUsernameInput
		}
		if found == visibleUsernameInput {
			return uiauto.Combine("MS SignIn Retry Username",
				ms.ChooseSearchEngine(),
				ms.ui.WaitUntilExists(visibleUsernameInput),
				ms.ui.LeftClickUntilFocused(usernameInput),
				ms.kb.TypeAction(userName),
				ms.kb.AccelAction("Enter"),
				ms.ui.WaitUntilGone(visibleUsernameInput),
			)(ctx)
		}
		testing.ContextLog(ctx, "Microsoft OAuth retry skipped username prompt")
		return nil
	}
}

// InputPassword waits for the Microsoft "input password" screen and input the password.
// If an active Microsoft session skips the password screen and lands directly on a post-password
// screen, InputPassword returns nil cleanly.
func (ms *Ms365) InputPassword(password string) uiauto.Action {
	return ms.inputPassword(password, nil, nil)
}

func (ms *Ms365) inputPassword(password string, setupCompleteDialogFinder, abortFinder *nodewith.Finder) uiauto.Action {
	msPasswordWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Enter your password)|(Get a code to sign in)|(Other ways to sign in)|(Choose a way to sign in)|(Verify your identity)")).First()
	passwordInput := nodewith.Ancestor(msPasswordWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(p|P)assword.*"))
	visiblePasswordInput := passwordInput.Visible()
	otherWaysButton := nodewith.Ancestor(msPasswordWindow).Role(role.Button).NameRegex(regexp.MustCompile("(?i)Other ways to sign in.*")).First()
	useYourPasswordInsteadButton := nodewith.Ancestor(msPasswordWindow).NameRegex(regexp.MustCompile("(?i).*Use your password.*")).First()

	msStaySignedInWindowName := regexp.MustCompile("(Microsoft account|Sign in to your( Microsoft)? account|" + regexp.QuoteMeta("Stay signed in?") + ")")
	msStaySignedInWindow := nodewith.Role(role.RootWebArea).NameRegex(msStaySignedInWindowName)
	msStaySignedInRoot := nodewith.Role(role.RootWebArea).Name("Stay signed in?").First()
	msStaySignedInHeading := nodewith.Ancestor(msStaySignedInWindow).Role(role.Heading).Name("Stay signed in?").First()
	msUpdateTermsWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i).*updating our terms.*")).First()
	msConfirmSignInWindow := nodewith.Role(role.RootWebArea).Name("Is your security info still accurate?")
	msAcceptPermissionWindow := nodewith.Role(role.RootWebArea).NameContaining("Let this app access your info?")
	msSetupOrConnectedDialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Microsoft OneDrive connected|Microsoft 365 setup complete)"))

	finders := []*nodewith.Finder{
		visiblePasswordInput,
		otherWaysButton,
		useYourPasswordInsteadButton,
		msUpdateTermsWindow,
		msConfirmSignInWindow,
		msStaySignedInRoot,
		msStaySignedInHeading,
		msAcceptPermissionWindow,
		msSetupOrConnectedDialog,
	}
	if setupCompleteDialogFinder != nil {
		finders = append(finders, setupCompleteDialogFinder)
	}
	if abortFinder != nil {
		finders = append(finders, abortFinder)
	}

	return func(ctx context.Context) error {
		found, err := ms.ui.FindAnyExists(ctx, finders...)
		if err != nil {
			return errors.Wrap(err, "failed to find password input or post-password screen")
		}
		if abortFinder != nil && found == abortFinder {
			return errOneDriveRetryNeeded
		}
		if setupCompleteDialogFinder != nil && found == setupCompleteDialogFinder {
			testing.ContextLog(ctx, "Microsoft login completed without password prompt")
			return nil
		}
		switch found {
		case msUpdateTermsWindow, msConfirmSignInWindow, msStaySignedInRoot, msStaySignedInHeading, msAcceptPermissionWindow, msSetupOrConnectedDialog:
			testing.ContextLog(ctx, "Microsoft login skipped password prompt; proceeding to post-password screens")
			return nil
		case otherWaysButton:
			if err := ms.ui.LeftClickUntil(otherWaysButton, ms.ui.Exists(useYourPasswordInsteadButton))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Other ways to sign in'")
			}
			if err := ms.ui.LeftClickUntil(useYourPasswordInsteadButton, ms.ui.Exists(visiblePasswordInput))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Use your password'")
			}
		case useYourPasswordInsteadButton:
			if err := ms.ui.LeftClickUntil(useYourPasswordInsteadButton, ms.ui.Exists(visiblePasswordInput))(ctx); err != nil {
				return errors.Wrap(err, "failed to click 'Use your password'")
			}
		}

		return uiauto.Combine("MS SignIn Password",
			ms.ui.WaitUntilExists(visiblePasswordInput),
			ms.ui.LeftClickUntilFocused(passwordInput),
			ms.kb.TypeAction(password),
			ms.kb.AccelAction("Enter"),
			ms.ui.WaitUntilGone(visiblePasswordInput),
		)(ctx)
	}
}

// DoNotSavePassword proceeds through optional "Save password?" Chrome popup shown after entering password.
func (ms *Ms365) DoNotSavePassword() uiauto.Action {
	chromeSavePasswordWindow := nodewith.Role(role.Window).Name("Save password?")
	chromeNeverSaveButton := nodewith.Ancestor(chromeSavePasswordWindow).Role(role.Button).Name("Never")

	return func(ctx context.Context) error {
		if err := ms.ui.EnsureGoneFor(chromeSavePasswordWindow, 5*time.Second)(ctx); err != nil {
			return ms.ui.LeftClickUntil(chromeNeverSaveButton, ms.ui.Gone(chromeNeverSaveButton))(ctx)
		}
		return nil
	}
}

// ConfirmSignIn proceeds through optional Microsoft security information dialog shown after entering login.
func (ms *Ms365) ConfirmSignIn() uiauto.Action {
	msConfirmSignInWindow := nodewith.Role(role.RootWebArea).Name("Is your security info still accurate?")
	msConfirmSignInButton := nodewith.Ancestor(msConfirmSignInWindow).Name("Looks good!").First()

	return func(ctx context.Context) error {
		if err := ms.ui.EnsureGoneFor(msConfirmSignInWindow, 5*time.Second)(ctx); err != nil {
			return ms.ui.LeftClickUntil(msConfirmSignInButton, ms.ui.Gone(msConfirmSignInButton))(ctx)
		}
		return nil
	}
}

// handlePostPasswordFlow advances through post-password prompts ("Save password?",
// "We're updating our terms", "Is your security info still accurate?", and "Stay signed in?")
// until Microsoft consent or the ChromeOS setup/connected dialog is reached.
func (ms *Ms365) handlePostPasswordFlow(setupCompleteDialogFinder, abortFinder *nodewith.Finder) uiauto.Action {
	chromeSavePasswordWindow := nodewith.Role(role.Window).Name("Save password?")
	chromeNeverSaveButton := nodewith.Ancestor(chromeSavePasswordWindow).Role(role.Button).Name("Never")

	msUpdateTermsWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i).*updating our terms.*")).First()
	msUpdateTermsNextButton := nodewith.Ancestor(msUpdateTermsWindow).Role(role.Button).Name("Next").First()

	msConfirmSignInWindow := nodewith.Role(role.RootWebArea).Name("Is your security info still accurate?")
	msConfirmSignInButton := nodewith.Ancestor(msConfirmSignInWindow).Name("Looks good!").First()

	msStaySignedInWindowName := regexp.MustCompile("(Microsoft account|Sign in to your( Microsoft)? account|" + regexp.QuoteMeta("Stay signed in?") + ")")
	msStaySignedInWindow := nodewith.Role(role.RootWebArea).NameRegex(msStaySignedInWindowName)
	msStaySignedInYesButton := nodewith.Ancestor(msStaySignedInWindow).Role(role.Button).Name("Yes").First()
	msStaySignedInYesText := nodewith.Ancestor(msStaySignedInWindow).Role(role.StaticText).Name("Yes").First()

	msAcceptPermissionWindow := nodewith.Role(role.RootWebArea).NameContaining("Let this app access your info?")

	if setupCompleteDialogFinder == nil {
		setupCompleteDialogFinder = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Microsoft OneDrive connected|Microsoft 365 setup complete)"))
	}

	finders := []*nodewith.Finder{
		chromeNeverSaveButton,
		msAcceptPermissionWindow,
		setupCompleteDialogFinder,
		msUpdateTermsNextButton,
		msConfirmSignInButton,
		msStaySignedInYesButton,
		msStaySignedInYesText,
	}
	if abortFinder != nil {
		finders = append(finders, abortFinder)
	}

	return func(ctx context.Context) error {
		for step := 0; step < 8; step++ {
			found, err := ms.ui.FindAnyExists(ctx, finders...)
			if err != nil {
				return errors.Wrap(err, "failed waiting for Microsoft post-password screen")
			}
			if abortFinder != nil && found == abortFinder {
				return errOneDriveRetryNeeded
			}

			switch found {
			case msAcceptPermissionWindow, setupCompleteDialogFinder:
				return nil
			case chromeNeverSaveButton:
				testing.ContextLog(ctx, "Dismissing Chrome 'Save password?' prompt")
				if err := ms.ui.WithTimeout(5*time.Second).LeftClickUntil(chromeNeverSaveButton, ms.ui.Gone(chromeNeverSaveButton))(ctx); err != nil {
					if ms.ui.Exists(chromeNeverSaveButton)(ctx) != nil {
						testing.ContextLog(ctx, "Chrome 'Save password?' prompt disappeared before click completed")
						continue
					}
					return errors.Wrap(err, "failed to dismiss 'Save password?' prompt")
				}
			case msUpdateTermsNextButton:
				testing.ContextLog(ctx, "Accepting Microsoft 'We're updating our terms' interstitial")
				if err := uiauto.Combine("Accept MS updated terms",
					ms.ui.ScrollToVisible(msUpdateTermsNextButton),
					ms.ui.LeftClickUntil(msUpdateTermsNextButton, ms.ui.Gone(msUpdateTermsNextButton)),
					ms.ui.WaitUntilGone(msUpdateTermsWindow),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to accept 'We're updating our terms'")
				}
			case msConfirmSignInButton:
				testing.ContextLog(ctx, "Confirming Microsoft security info dialog")
				if err := uiauto.Combine("Confirm MS security info",
					ms.ui.ScrollToVisible(msConfirmSignInButton),
					ms.ui.LeftClickUntil(msConfirmSignInButton, ms.ui.Gone(msConfirmSignInButton)),
					ms.ui.WaitUntilGone(msConfirmSignInWindow),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to confirm Microsoft security info")
				}
			case msStaySignedInYesButton, msStaySignedInYesText:
				testing.ContextLog(ctx, "Clicking Yes on Microsoft 'Stay signed in?' prompt")
				if err := uiauto.Combine("MS Stay Signed In",
					ms.ui.ScrollToVisible(found),
					ms.ui.LeftClickUntil(found, ms.ui.Gone(found)),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to click Yes on 'Stay signed in?'")
				}
			}
		}
		return errors.New("exceeded maximum post-password interstitial steps during Microsoft login")
	}
}

// StaySignedIn waits for the Microsoft "Stay signed in?" screen (and any preceding interstitials) and clicks Yes.
func (ms *Ms365) StaySignedIn() uiauto.Action {
	return ms.handlePostPasswordFlow(nil, nil)
}

// AcceptPermissionIfNeeded waits for the optional Microsoft "Access permission" screen and clicks to accept it.
// It also detects the cloud_upload.SetupComplete dialog to indicate that the permission screen wasn't presented at all.
func (ms *Ms365) AcceptPermissionIfNeeded(setupCompleteDialogFinder *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		if setupCompleteDialogFinder == nil {
			setupCompleteDialogFinder = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Microsoft OneDrive connected|Microsoft 365 setup complete)"))
		}
		msAcceptPermissionWindow := nodewith.Role(role.RootWebArea).NameContaining("Let this app access your info?")
		msAcceptPermissionButton := nodewith.Ancestor(msAcceptPermissionWindow).Role(role.Button).NameRegex(regexp.MustCompile("^(Accept|Yes)$")).First()
		runAcceptPermission := uiauto.Combine("MS permission screen",
			ms.ui.WaitUntilExists(msAcceptPermissionButton),
			ms.ui.ScrollToVisible(msAcceptPermissionButton),
			ms.kb.TypeKeyAction(input.KEY_END),
			ms.ui.LeftClickUntil(msAcceptPermissionButton, ms.ui.Gone(msAcceptPermissionButton)),
		)

		// When the permission dialog doesn't show it goes directly to Setup Complete dialog.
		found, err := ms.ui.FindAnyExists(ctx, msAcceptPermissionWindow, setupCompleteDialogFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find the MS accept permission window")
		}
		if found == msAcceptPermissionWindow {
			if err := runAcceptPermission(ctx); err != nil {
				return err
			}
			return ms.ui.WaitUntilExists(setupCompleteDialogFinder)(ctx)
		}
		// `setupCompleteDialog` has been found, nothing to do.
		return nil
	}
}

// LoginToMicrosoft365 logs into the Microsoft 365 app with the username and password in the instance.
// If accepting Microsoft account interstitials (such as updated terms or security-info reminders)
// causes chrome.identity.launchWebAuthFlow to abort with "Can't connect to OneDrive. Try again.",
// it clicks "Connect to OneDrive" and retries up to twice (3 total attempts).
func (ms *Ms365) LoginToMicrosoft365(setupCompleteDialogFinder *nodewith.Finder) uiauto.Action {
	const maxLoginAttempts = 3

	connectToOneDriveDialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile(`Connect to Microsoft OneDrive.*`))
	connectOneDriveError := nodewith.Ancestor(connectToOneDriveDialog).Role(role.StaticText).Name("Can't connect to OneDrive. Try again.").Visible()
	retryConnectButton := nodewith.Ancestor(connectToOneDriveDialog).Role(role.Button).Name("Connect to OneDrive")
	focusedRetryConnectButton := retryConnectButton.Focused()

	return func(ctx context.Context) error {
		for attempt := 0; attempt < maxLoginAttempts; attempt++ {
			var abortFinder *nodewith.Finder
			if attempt == 0 {
				abortFinder = connectOneDriveError
			} else if attempt < maxLoginAttempts-1 {
				abortFinder = focusedRetryConnectButton
			}

			usernameStep := ms.inputUserName(ms.UserName, abortFinder)
			if attempt > 0 {
				usernameStep = ms.inputUserNameOnRetry(ms.UserName, setupCompleteDialogFinder, abortFinder)
			}

			err := uiauto.Combine("Login to Microsoft 365",
				usernameStep,
				ms.inputPassword(ms.Password, setupCompleteDialogFinder, abortFinder),
				ms.handlePostPasswordFlow(setupCompleteDialogFinder, abortFinder),
				ms.AcceptPermissionIfNeeded(setupCompleteDialogFinder),
			)(ctx)
			if !errors.Is(err, errOneDriveRetryNeeded) || attempt == maxLoginAttempts-1 {
				return err
			}

			testing.ContextLogf(ctx, "OneDrive OAuth flow aborted after Microsoft interstitial (attempt %d/%d); clicking 'Connect to OneDrive' to retry", attempt+1, maxLoginAttempts)
			if err := uiauto.Combine("Click Connect to OneDrive to retry",
				ms.ui.WaitUntilExists(retryConnectButton),
				ms.ui.LeftClick(retryConnectButton),
				ms.ui.WithTimeout(15*time.Second).WaitUntilGone(focusedRetryConnectButton),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to launch retry OneDrive OAuth popup")
			}
		}
		return nil
	}
}

// Microsoft365WindowFinder is a finder for a window of Microsoft 365 (Word, Excel or PowerPoint)
// opened for the given fileName.
func Microsoft365WindowFinder(fileName string) *nodewith.Finder {
	return nodewith.Role(role.Window).NameContaining(fileName).HasClass("WebContentsViewAura")
}

// Microsoft365WindowFinderWithRegex is a finder for a window of Microsoft 365 (Word, Excel or PowerPoint)
// opened for the given file name regex.
func Microsoft365WindowFinderWithRegex(fileNameRegex *regexp.Regexp) *nodewith.Finder {
	return nodewith.Role(role.Window).NameRegex(fileNameRegex).HasClass("WebContentsViewAura")
}

// closeMicrosoft365Window finds the Microsoft 365 PWA window with the specific
// file name and closes it.
// On ChromeOS R157+, Word, Excel, and PowerPoint Online serve an empty
// <meta name="application-title" content=""> tag, causing ash.Window.Title
// (BrowserWidget) to be just "Microsoft 365" without fileName, while
// BrowserRootView and WebContentsViewAura still include fileName.
func closeMicrosoft365Window(ctx context.Context, tconn *chrome.TestConn, fileName string) error {
	ui := uiauto.New(tconn)
	browserRootView := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileName).First()
	webAppToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(browserRootView)
	closeButton := nodewith.Role(role.Button).Name("Close").HasClass("FrameCaptionButton").Ancestor(browserRootView)

	if err := ui.WithTimeout(defaultUITimeout).WaitUntilExists(webAppToolbar)(ctx); err != nil {
		return errors.Wrapf(err, "failed to find MS365 PWA window for %q", fileName)
	}
	if err := ui.WithTimeout(defaultUITimeout).LeftClickUntil(closeButton, ui.Gone(Microsoft365WindowFinder(fileName)))(ctx); err != nil {
		return errors.Wrapf(err, "failed to close MS365 PWA window for %q", fileName)
	}
	if err := ui.WithTimeout(defaultUITimeout).WaitUntilGone(browserRootView)(ctx); err != nil {
		return errors.Wrapf(err, "failed waiting for MS365 BrowserRootView %q to close", fileName)
	}
	return nil
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

// waitForMicrosoft365EditorWindowAndClose waits for the Microsoft 365 app window to open and close it.
// If "ignoreCloseError" is true, it will just ignore the error, otherwise it returns error if the closure fails,.
func (ms *Ms365) waitForMicrosoft365EditorWindowAndClose(tconn *chrome.TestConn, fileName string, ignoreCloseError bool) uiauto.Action {
	return func(ctx context.Context) error {
		ms365App := Microsoft365WindowFinder(fileName)
		msWebSignIn := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(^Sign in$)|(Sign in to .*)|(Pick an account)|(Enter your password)|(Get a code to sign in)|(Verify your identity)")).First()

		found, err := ms.ui.WithTimeout(time.Minute).FindAnyExists(ctx, ms365App, msWebSignIn)
		if err != nil {
			return errors.Wrap(err, "failed to wait for the MS365 window")
		}
		if found == msWebSignIn && ms.UserName != "" && ms.Password != "" {
			testing.ContextLogf(ctx, "Signing into Microsoft web prompt while opening %q", fileName)
			if err := uiauto.Combine("Sign in to Microsoft 365 web editor",
				ms.inputUserNameOnRetry(ms.UserName, ms365App, nil),
				ms.inputPassword(ms.Password, ms365App, nil),
				ms.handlePostPasswordFlow(ms365App, nil),
				ms.AcceptPermissionIfNeeded(ms365App),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to sign in to Microsoft 365 web prompt")
			}
			if err := ms.ui.WithTimeout(time.Minute).WaitUntilExists(ms365App)(ctx); err != nil {
				return errors.Wrap(err, "failed to wait for the MS365 window after sign-in")
			}
		} else if found != ms365App {
			if err := ms.ui.WithTimeout(time.Minute).WaitUntilExists(ms365App)(ctx); err != nil {
				return errors.Wrap(err, "failed to wait for the MS365 window")
			}
		}

		// Check that MS365 is showing the Word, Excel, or PowerPoint editor or viewer.
		editorRootView := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileName).First()
		if _, err := ms.ui.FindAnyExists(ctx,
			nodewith.Attribute("htmlId", "EditorContainer").Ancestor(editorRootView).First(),
			nodewith.Role(role.GenericContainer).Name("Document Contents").Ancestor(editorRootView).First(),
			nodewith.Role(role.Toolbar).Name("Ribbon").Ancestor(editorRootView).First(),
			nodewith.Role(role.Toolbar).Name("Header").Ancestor(editorRootView).First(),
			nodewith.Role(role.Image).HasClass("WACPageImg").Ancestor(editorRootView).First(),
		); err != nil {
			return errors.Wrap(err, "failed to find MS365 editor content")
		}
		if err := closeMicrosoft365Window(ctx, tconn, fileName); err != nil {
			if !ignoreCloseError {
				return errors.Wrap(err, "failed to close the MS365 window")
			}
		}
		return nil
	}
}

// WaitForMicrosoft365EditorWindowAndClose closes the MS window and guarantees that
// the window is closed successfully, it will return errors if the closure fails.
func (ms *Ms365) WaitForMicrosoft365EditorWindowAndClose(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return ms.waitForMicrosoft365EditorWindowAndClose(tconn, fileName, false /*=ignoreCloseError*/)
}

// WaitForMicrosoft365EditorWindowAndCloseIgnoreError tries to close the MS window but won't
// return error if it fails. This is used mostly when closing window is the last
// step.
func (ms *Ms365) WaitForMicrosoft365EditorWindowAndCloseIgnoreError(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return ms.waitForMicrosoft365EditorWindowAndClose(tconn, fileName, true /*=ignoreCloseError*/)
}

// InstallPWA installs Office PWA.
func (ms *Ms365) InstallPWA(ctx context.Context, cr *chrome.Chrome) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	conn, err := cr.NewConn(ctx, officePWAInstallURL)
	if err != nil {
		return errors.Wrap(err, "failed to open office website")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	window := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile(".*Microsoft 365.*"))
	installIcon := nodewith.ClassName("PwaInstallView").Role(role.Button)
	installButton := nodewith.Name("Install").Role(role.Button)

	// Somewhat hacky, but we install the PWA before logging in, as it's the only
	// way to stop the site from redirecting from microsoft365.com, where the
	// web app manifest is linked in the page (via ODFS), vs newer domains where
	// it's not and installation via omnibox isn't possible.
	if err := uiauto.Combine("Install Office PWA through omnibox",
		ms.ui.WaitUntilExists(window),
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

// ClearBrowserCookiesForOffice allows popups for Office MSAL sign-in flows and
// clears all non-Google browser cookies and web storage.
func ClearBrowserCookiesForOffice(ctx context.Context, cr *chrome.Chrome) error {
	testing.ContextLog(ctx, "Clearing cookies and web storage for Office and Microsoft login websites")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get Test API connection")
	}

	const clearExpr = `(async () => {
		await tast.promisify(tast.bind(chrome.contentSettings.popups, 'set'))({
			primaryPattern: '<all_urls>',
			setting: 'allow',
		});
		await tast.promisify(tast.bind(chrome.browsingData, 'remove'))({
			excludeOrigins: [
				'https://accounts.google.com',
				'https://www.google.com',
				'https://drive.google.com',
				'https://docs.google.com',
				'https://apis.google.com',
				'https://myaccount.google.com',
			],
		}, {
			cacheStorage: true,
			cookies: true,
			fileSystems: true,
			indexedDB: true,
			localStorage: true,
			serviceWorkers: true,
			webSQL: true,
		});
	})()`

	if err := tconn.Eval(ctx, clearExpr, nil); err != nil {
		return errors.Wrap(err, "failed to clear non-Google browsing data")
	}
	return nil
}
