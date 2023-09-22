// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package imetestutil

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// InputActionData represents a set of keyboard actions and their expected outcomes.
type InputActionData struct {
	Actions    uiauto.Action
	TestString string
}

// CheckInputViaClipboard copies all text from the currently focused input text field and checks the clipboard content for a matching string.
// The input field must be focused before this function is called.
// Note that the cursor will be moved to the end of the input field before returning.
func CheckInputViaClipboard(ctx context.Context, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, expectedText string) error {
	// Make sure the clipboard is empty.
	if err := ash.SetClipboard(ctx, tconn, ""); err != nil {
		return errors.Wrap(err, "failed to clear clipboard")
	}

	if err := uiauto.Combine("copy text to clipboard",
		// Select all the text in the input box.
		keyboard.AccelAction("ctrl+A"),
		// Copy selected content.
		keyboard.AccelAction("ctrl+C"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy input to clipboard")
	}

	if err := ash.WaitUntilClipboardText(ctx, tconn, expectedText); err != nil {
		return err
	}

	// Move cursor to the end of the selected text.
	if err := keyboard.AccelAction("Right")(ctx); err != nil {
		return errors.Wrap(err, "failed to deselect text")
	}
	return nil
}

// TestInputActionsInEditor tests basic string input and editing in English in an editor app (VSCode, gedit).
// It assumes that the editor is already opened and focused for entering test strings.
func TestInputActionsInEditor(ctx context.Context, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn) error {
	inputActions := getBasicInputActions(ctx, keyboard)
	editorTabActions := []InputActionData{
		{uiauto.Combine("insert Tab key at start",
			// |Hi\nHi\n\nThis is a
			keyboard.AccelAction("Ctrl+A"),
			keyboard.AccelAction("Left"),
			keyboard.AccelAction("Tab"),
		),
			"\tHi\nHi\n\nThis is a ",
		},
		{uiauto.Combine("insert Tab key at start of all lines",
			// |\tHi\n\tHi\n\t\n\tThis is a|
			keyboard.AccelAction("Ctrl+A"),
			keyboard.AccelAction("Tab"),
			keyboard.AccelAction("Left"),
		),
			"\t\tHi\n\tHi\n\n\tThis is a ",
		},
	}
	inputActions = append(inputActions, editorTabActions...)
	return runInputActions(ctx, keyboard, tconn, inputActions)
}

// TestInputActionsInFirefox tests basic string input and editing in English in the firefox app.
// It assumes Firefox is launched with a test page containing 2 input fields, and the first input box is focused.
func TestInputActionsInFirefox(ctx context.Context, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn) error {
	inputActions := getBasicInputActions(ctx, keyboard)
	firefoxTabActions := []InputActionData{
		{uiauto.Combine("test tab functionality",
			// Press tab to move focus to a separate input field.
			keyboard.AccelAction("Tab"),
			// Type the new string.
			keyboard.TypeAction("Separate input"),
		),
			// Only the new string should exist in the new input field.
			"Separate input",
		},
	}
	// The first input box is used to test basic inputs, then tab to the second input box to check tab behavior.
	inputActions = append(inputActions, firefoxTabActions...)
	return runInputActions(ctx, keyboard, tconn, inputActions)
}

// getBasicInputActions returns an array of InputActions to enter a test string in English and uses a range of non-alphanumeric keys to edit the text.
func getBasicInputActions(ctx context.Context, keyboard *input.KeyboardEventWriter) []InputActionData {
	return []InputActionData{
		{uiauto.Combine("type the initial 2-line string",
			keyboard.TypeAction("Hi\nThis is a test."),
		),
			"Hi\nThis is a test.",
		},
		{uiauto.Combine("go left and delete one character",
			// Hi\nThis is a test|.
			keyboard.AccelAction("Left"),
			keyboard.AccelAction("Backspace"),
		),
			"Hi\nThis is a tes.",
		},
		{uiauto.Combine("go left and delete multiple characters",
			// Hi\nThis is a |tes.|
			keyboard.AccelAction("Shift+Left"),
			keyboard.AccelAction("Shift+Left"),
			keyboard.AccelAction("Shift+Left"),
			keyboard.AccelAction("Shift+Left"),
			// Hi\nThis is a |
			keyboard.AccelAction("Backspace"),
		),
			"Hi\nThis is a ",
		},
		{uiauto.Combine("use arrow keys to copy the first line of string, then go up and paste",
			// |Hi\nThis is a
			keyboard.AccelAction("Ctrl+A"),
			keyboard.AccelAction("Left"),
			// |Hi\n|This is a
			keyboard.AccelAction("Shift+Down"),
			// |Hi\nThis is a
			keyboard.AccelAction("Ctrl+C"),
			keyboard.AccelAction("Up"),
			// Hi\nHi\n|This is a
			keyboard.AccelAction("Ctrl+V"),
		),
			"Hi\nHi\nThis is a ",
		},
		{uiauto.Combine("enter keys",
			// Hi\nHi|\nThis is a
			keyboard.AccelAction("Shift+Up"),
			keyboard.AccelAction("Left"),
			// Hi\nHi\n|\nThis is a
			keyboard.AccelAction("Enter"),
		),
			"Hi\nHi\n\nThis is a ",
		},
	}
}

// runInputActions takes in an array of app-specific inputActions and performs them, then uses the clipboard to check if the result is
// It assumes that an input field is already focused.
func runInputActions(ctx context.Context, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, inputActions []InputActionData) error {
	// Switch to EnglishUS IME
	if err := ime.EnglishUS.InstallAndActivate(tconn)(ctx); err != nil {
		return err
	}

	for _, inputAction := range inputActions {
		testing.ContextLogf(ctx, "Typing %q", inputAction.TestString)
		if err := inputAction.Actions(ctx); err != nil {
			return errors.Wrap(err, "failed to complete keyboard action ")
		}
		if err := CheckInputViaClipboard(ctx, keyboard, tconn, inputAction.TestString); err != nil {
			return errors.Wrap(err, "failed to check current text")
		}
	}

	return nil
}
