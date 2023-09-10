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
		termTimeout            = 30 * time.Second
		anyDialogTimeout       = 5 * time.Second
		anyActionButtonTimeout = 1 * time.Minute
	)
	consolidatedConsentHeader := nodewith.Name("Review these terms and control your data").Role(role.Dialog)
	if err := ui.WithTimeout(termTimeout).WaitUntilExists(consolidatedConsentHeader)(ctx); err != nil {
		return err
	}

	// In lower resolution screens, a `see more` button is shown and the accept button is hidden until the `see more` button is clicked.
	acceptAndContinue := nodewith.Name("Accept and continue").Role(role.Button)
	focusedButton := nodewith.State(state.Focused, true).Role(role.Button)

	if err := uiauto.IfSuccessThen(
		ui.WaitUntilExists(focusedButton),
		ui.LeftClickUntil(focusedButton, ui.WaitUntilExists(acceptAndContinue)))(ctx); err != nil {
		return err
	}

	if err := uiauto.Combine("accept and continue",
		ui.WaitUntilExists(acceptAndContinue),
		ui.LeftClickUntil(acceptAndContinue, ui.Gone(acceptAndContinue)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to accept terms")
	}
	testing.ContextLog(ctx, "finish accept and continue")

	anyDialog := nodewith.First().Role(role.Dialog)
	anyActionButton := nodewith.NameRegex(regexp.MustCompile(
		"Skip|" +
			"No thanks|" +
			"Next|" +
			"Accept and continue|" +
			"Turn on sync|" +
			"Get started|" +
			"Close")).First().Role(role.Button)

	lastActionTime := time.Now()
	for {
		if err := ui.Exists(anyActionButton)(ctx); err == nil {
			// Some action button is detected. Click it
			testing.ContextLog(ctx, "Detected action button")
			if err := ui.LeftClickUntil(anyActionButton, ui.Gone(anyActionButton))(ctx); err != nil {
				return errors.Wrap(err, "failed to click button")
			}

			testing.ContextLog(ctx, "Action button has been clicked")
			lastActionTime = time.Now()
			continue
		}

		if err := ui.WithTimeout(anyDialogTimeout).WaitUntilExists(anyDialog)(ctx); err == nil {
			// Some dialog is still shown.
			if time.Since(lastActionTime) > anyActionButtonTimeout {
				return errors.New("failed to detect action button")
			}
			continue
		}

		// Double sure any dialog is gone.
		if err := ui.Gone(anyDialog)(ctx); err != nil {
			return errors.Wrap(err, "failed to confirm dialog is gone")
		}

		break
	}

	return nil
}
