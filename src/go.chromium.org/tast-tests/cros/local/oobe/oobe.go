// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package oobe contains helpers for shared logic in OOBE related tests.
package oobe

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// IsWelcomeScreenVisible checks the current page in OOBE to see if it's
// currently on the welcome page.
func IsWelcomeScreenVisible(ctx context.Context, oobeConn *chrome.Conn) error {
	return oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()")
}

// IsHidDetectionScreenVisible checks if the current page in OOBE to see if it's
// currently on the HID Detection page.
func IsHidDetectionScreenVisible(ctx context.Context, oobeConn *chrome.Conn) error {
	return oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.HIDDetectionScreen.isVisible()")
}

// IsHidDetectionTouchscreenDetected checks if a touchscreen is detected in the
// OOBE HID Detection page.
func IsHidDetectionTouchscreenDetected(ctx context.Context, oobeConn *chrome.Conn) error {
	return oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.HIDDetectionScreen.touchscreenDetected()")
}

// IsHidDetectionContinueButtonEnabled checks if the continue button is enabled
// in the OOBE HID Detection page.
func IsHidDetectionContinueButtonEnabled(ctx context.Context, oobeConn *chrome.Conn) error {
	return oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.HIDDetectionScreen.canClickNext()")
}

// IsHidDetectionContinueButtonDisabled checks if the continue button is disabled
// in the OOBE HID Detection page.
func IsHidDetectionContinueButtonDisabled(ctx context.Context, oobeConn *chrome.Conn) error {
	return oobeConn.WaitForExprFailOnErr(ctx, "!OobeAPI.screens.HIDDetectionScreen.canClickNext()")
}

// IsHidDetectionSearchingForKeyboard checks if OOBE HID Detection page is searching for keyboard device.
func IsHidDetectionSearchingForKeyboard(ctx context.Context, oobeConn *chrome.Conn, tconn *chrome.TestConn) error {
	var keyboardNotDetectedText string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.HIDDetectionScreen.getKeyboardNotDetectedText()", &keyboardNotDetectedText); err != nil {
		return err
	}
	keyboardNotDetectedTextNode := nodewith.Role(role.StaticText).Name(keyboardNotDetectedText)
	return uiauto.New(tconn).WaitUntilExists(keyboardNotDetectedTextNode)(ctx)
}

// IsHidDetectionSearchingForMouse checks if OOBE HID Detection page is searching for mouse device.
func IsHidDetectionSearchingForMouse(ctx context.Context, oobeConn *chrome.Conn, tconn *chrome.TestConn) error {
	var mouseNotDetectedText string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.HIDDetectionScreen.getMouseNotDetectedText()", &mouseNotDetectedText); err != nil {
		return err
	}
	mouseNotDetectedTextNode := nodewith.Role(role.StaticText).Name(mouseNotDetectedText)
	return uiauto.New(tconn).WaitUntilExists(mouseNotDetectedTextNode)(ctx)
}

// ClickHidScreenNextButton clicks on the next button in OOBE HID detection screen.
func ClickHidScreenNextButton(ctx context.Context, oobeConn *chrome.Conn, tconn *chrome.TestConn) error {
	var nextButtonName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.HIDDetectionScreen.getNextButtonName()", &nextButtonName); err != nil {
		return errors.Wrap(err, "failed to retrieve the next button")
	}

	nextButtonFinder := nodewith.Name(nextButtonName).Role(role.Button)
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	if err := uiauto.Combine("Click on next button",
		ui.WaitUntilExists(nextButtonFinder),
		ui.LeftClick(nextButtonFinder),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on next button")
	}

	return nil
}

// IsHidDetectionMouseDetected checks if there is no keyboard detected in
// the OOBE HID Detection page.
func IsHidDetectionMouseDetected(ctx context.Context, oobeConn *chrome.Conn, tconn *chrome.TestConn) error {
	var mouseDetectedText string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.HIDDetectionScreen.mouseDetected()", &mouseDetectedText); err != nil {
		return err
	}
	mouseDetectedTextNode := nodewith.Role(role.StaticText).Name(mouseDetectedText)
	return uiauto.New(tconn).WaitUntilExists(mouseDetectedTextNode)(ctx)
}

// AdvanceThroughConsolidatedConsentIfShown checks whether the consolidated screen is shown.
// If the consolidated screen is shown, wait for it to load, click the accept button in the
// consolidated consent screen after clicking the read more button if it's shown.
func AdvanceThroughConsolidatedConsentIfShown(ctx context.Context, oobeConn *chrome.Conn, tconn *chrome.TestConn) error {
	var shouldSkipConsolidatedConsentScreen bool
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.shouldSkip()", &shouldSkipConsolidatedConsentScreen); err != nil {
		return errors.Wrap(err, "failed to evaluate whether to skip consolidated consent screen")
	}
	if shouldSkipConsolidatedConsentScreen {
		return nil
	}

	isReadMoreButtonShown := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.isReadMoreButtonShown()", &isReadMoreButtonShown); err != nil {
		return errors.Wrap(err, "failed to evaluate whether the consolidated consent screen read more button is shown")
	}

	ui := uiauto.New(tconn).WithTimeout(100 * time.Second)
	focusedButton := nodewith.State(state.Focused, true).Role(role.Button)
	if isReadMoreButtonShown {
		if err := uiauto.Combine("click the consolidated consent screen read more button",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to click the consolidated consent screen read more button")
		}
		if err := oobeConn.WaitForExprFailOnErr(ctx, "!OobeAPI.screens.ConsolidatedConsentScreen.isReadMoreButtonShown()"); err != nil {
			return errors.Wrap(err, "failed to wait for the consolidated consent read more to be hidden")
		}
	}

	if err := uiauto.Combine("Click the consolidated consent screen accept button",
		ui.WaitUntilExists(focusedButton),
		ui.LeftClick(focusedButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the consolidated consent screen accept button")
	}
	return nil
}

// CompleteOnboardingFlow function goes through the onboarding flow screens.
// TODO(crbug.com/1327981): Use OOBE test API
func CompleteOnboardingFlow(ctx context.Context, ui *uiauto.Context) error {
	const (
		termTimeout             = 60 * time.Second
		anyDialogTimeout        = 10 * time.Second
		anyActionButtonTimeout  = 1 * time.Minute
		acceptTimeout           = 3 * time.Second
		findActionButtonTimeout = 1 * time.Second
	)
	consolidatedConsentHeader := nodewith.Name("Review these terms and control your data").Role(role.Dialog)
	if err := ui.WithTimeout(termTimeout).WaitUntilExists(consolidatedConsentHeader)(ctx); err != nil {
		return err
	}

	// In lower resolution screens, a `see more` button is shown and the accept button is hidden until the `see more` button is clicked.
	// Note, focusedButton may be acceptAndContinue.
	acceptAndContinue := nodewith.Name("Accept and continue").Role(role.Button)
	focusedButton := nodewith.State(state.Focused, true).Role(role.Button)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.WithTimeout(acceptTimeout).WaitUntilExists(acceptAndContinue)(ctx); err == nil {
			return nil
		}

		if err := ui.WithTimeout(acceptTimeout).LeftClick(focusedButton)(ctx); err != nil {
			return testing.PollBreak(err)
		}

		return errors.New("Accept button not yet available")
	}, &testing.PollOptions{Timeout: termTimeout, Interval: time.Second}); err != nil {
		return err
	}

	if err := uiauto.Combine("accept and continue",
		ui.WaitUntilExists(acceptAndContinue),
		ui.LeftClickUntil(acceptAndContinue, ui.Gone(acceptAndContinue)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to accept terms")
	}
	testing.ContextLog(ctx, "finish accept and continue")

	loginDialog := nodewith.NameRegex(regexp.MustCompile(
		"Login Dialog")).First().Role(role.Window)

	useGoogleAccount := nodewith.Name(
		"Use Google Account password").Role(role.StaticText)

	nextButton := nodewith.Name("Next").First().Role(role.Button).Onscreen()

	actionButtons := []*nodewith.Finder{
		nodewith.Name("Skip").First().Role(role.Button).Onscreen(),
		nodewith.Name("No thanks").First().Role(role.Button).Onscreen(),
		nodewith.Name("Accept and continue").First().Role(role.Button).Onscreen(),
		nodewith.Name("Turn on sync").First().Role(role.Button).Onscreen(),
		nodewith.Name("Get started").First().Role(role.Button).Onscreen(),
		nodewith.Name("Close").First().Role(role.Button).Onscreen(),
		nextButton,
	}

	if err := ui.WithTimeout(anyDialogTimeout).WaitUntilExists(loginDialog)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait login dialog is shown")
	}

	lastActionTime := time.Now()
	for {
		if err := ui.Gone(loginDialog)(ctx); err == nil {
			testing.ContextLog(ctx, "Login dialog done")
			break
		}

		if time.Since(lastActionTime) > anyActionButtonTimeout {
			return errors.New("failed to detect action button")
		}

		if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(useGoogleAccount)(ctx); err == nil {
			testing.ContextLog(ctx, "Use google account found")
			if err := uiauto.NamedCombine("Select Use Google Account password and click next button",
				ui.DoDefault(useGoogleAccount),
				ui.DoDefaultUntil(nextButton, ui.Gone(nextButton)),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to use google account login")
			}
		}

		notDisabledPredicate := func(finder *nodewith.Finder) bool {
			return ui.CheckRestriction(finder, restriction.Disabled)(ctx) == nil
		}

		if actionButton, err := ui.WithTimeout(findActionButtonTimeout).FindAnyExistsMatching(ctx, notDisabledPredicate, "node disabled property is false", actionButtons...); err == nil {
			// Some action button is detected. Click it
			testing.ContextLogf(ctx, "Detected action button %s", actionButton.Pretty())
			if err := ui.DoDefault(actionButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to click button")
			}

			testing.ContextLogf(ctx, "Action button has been clicked: %s", actionButton.Pretty())
			lastActionTime = time.Now()
		}
	}
	return nil
}

// ProceedThroughGaiaInfoScreen clicks through the Gaia Info OOBE screen if shown.
func ProceedThroughGaiaInfoScreen(ctx context.Context, oobeConn *chrome.Conn) error {
	shouldSkipGaiaInfoScreen := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.GaiaInfoScreen.shouldSkip()", &shouldSkipGaiaInfoScreen); err != nil {
		return errors.Wrap(err, "failed to evaluate whether to skip Gaia Info screen")
	}

	if shouldSkipGaiaInfoScreen {
		return nil
	}

	testing.ContextLog(ctx, "Waiting for the Gaia Info screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GaiaInfoScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the gaia info screen to be visible")
	}

	// The Gaia Info Screen has two main UI states. When QuickStart is enabled, the user must choose between
	// the manual vs. QuickStart setup. When QuickStart is disabled, the only option is to click "Next".
	isQuickStartEnabled := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.GaiaInfoScreen.isOobeQuickStartEnabled()", &isQuickStartEnabled); err != nil {
		return errors.Wrap(err, "failed to evaluate whether QuickStart is enabled")
	}

	if isQuickStartEnabled {
		testing.ContextLog(ctx, "QuickStart is enabled, selecting manual setup on Gaia Info Screen")
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.GaiaInfoScreen.selectManualCredentials()", nil); err != nil {
			testing.ContextLog(ctx, "Unable to click gaia info screen manual credentials buttons: ", err)
		}
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.GaiaInfoScreen.clickNext()", nil); err != nil {
		return errors.Wrap(err, "failed to click gaia info screen next button")
	}
	return nil
}
