// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package oobe contains helpers for shared logic in OOBE related tests.
package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/errors"
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
