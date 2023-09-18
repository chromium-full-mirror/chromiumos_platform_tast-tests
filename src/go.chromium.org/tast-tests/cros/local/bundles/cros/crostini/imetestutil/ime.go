// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package imetestutil provides shared struct to test IME behaviour in Crostini.
package imetestutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PKCandidatesFinder is the finder for candidates in the IME candidates window.
// Since multiple candidates can appear, only use the first one.
var PKCandidatesFinder = nodewith.Role(role.ImeCandidate).Onscreen().First()

type enterInputActionPK func(keyboard *input.KeyboardEventWriter, ui *uiauto.Context) uiauto.Action

// IMETestData represents a set of inputs for ime testing.
type IMETestData struct {
	InputMethod             ime.InputMethod
	InputText               string
	ExpectedText            string
	EnterTestStringActionPK enterInputActionPK
}

type imeTestDataMap map[string]IMETestData

// IMETestCases containers a list of IMEs to run the app tests against. Used in params_test.go to generate test params.
var IMETestCases = imeTestDataMap{
	"english": IMETestData{
		ime.EnglishUS,
		"Hello",
		"Hello",
		func(keyboard *input.KeyboardEventWriter, ui *uiauto.Context) uiauto.Action {
			return keyboard.TypeAction("Hello")
		},
	},
	"japanese": IMETestData{
		ime.Japanese,
		"konnnitiha",
		"こんにちは",
		func(keyboard *input.KeyboardEventWriter, ui *uiauto.Context) uiauto.Action {
			return uiauto.Combine("Enter Japanese",
				setHiraganaMode(ui),
				keyboard.TypeAction("konnnitiha"),
				keyboard.AccelAction("Enter"),
				// Wait for the text to be committed.
				ui.WithTimeout(time.Second).WaitUntilGone(PKCandidatesFinder),
			)
		},
	},
	"arabic": IMETestData{
		ime.Arabic,
		"lvpfh",
		"مرحبا",
		func(keyboard *input.KeyboardEventWriter, ui *uiauto.Context) uiauto.Action {
			return keyboard.TypeAction("lvpfh")
		},
	},
}

func setHiraganaMode(ui *uiauto.Context) uiauto.Action {
	// The IME menu tray should exist by default when a new IMEs are added.
	imeMenuTrayButtonFinder := nodewith.Name("IME menu button").Role(role.Button)
	hiraganaOptionFinder := nodewith.Name("Hiragana").Role(role.CheckBox)
	// Set and check the hiragana mode is set.
	return uiauto.Combine("set hiragana mode",
		uiauto.IfFailThen(
			// If Hiragana is selected, skip to next step.
			uiauto.Combine("check that hiragana option is selected in the IME tray",
				ui.DoDefault(imeMenuTrayButtonFinder),
				ui.WaitUntilCheckedState(hiraganaOptionFinder, true),
			),
			// If Hiragana is not selected, attempt to select it a few times.
			uiauto.Retry(5,
				uiauto.Combine("set Japanese input to hiragana mode, and retry 5 times or until successful",
					ui.DoDefault(hiraganaOptionFinder),
					ui.WithTimeout(time.Second).WaitUntilGone(hiraganaOptionFinder),
					// Open and check again.
					ui.DoDefault(imeMenuTrayButtonFinder),
					ui.WaitUntilCheckedState(hiraganaOptionFinder, true),
				),
			),
		),
		// Hiragana is selected now, so click the menu to close the options.
		uiauto.Combine("close the input options",
			ui.DoDefault(imeMenuTrayButtonFinder),
			ui.WithTimeout(time.Second).WaitUntilGone(hiraganaOptionFinder),
		),
	)

}

// ResetToDefaultIME can be called to reset the input method back to default (EnglishUS) at the end of a test. If it fails, it will only log the error, and not return an error so the test will NOT fail.
func ResetToDefaultIME(ctx context.Context, tconn *chrome.TestConn) {
	if err := uiauto.Combine("reactivate default IME",
		ime.DefaultInputMethod.InstallAndActivate(tconn),
		ime.DefaultInputMethod.WaitUntilActivated(tconn),
	)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed switch back to default IME: ", err)
	}
}

// TestJapaneseCandidatesBoxInFirefox is a wrapper around TestJapaneseCandidatesBox.
// It moves the cursor to a new input field before entering the new test string, so that it is easier for uidetection to locate the string.
func TestJapaneseCandidatesBoxInFirefox(ctx context.Context, ui *uiauto.Context, uda *uidetection.Context, keyboard *input.KeyboardEventWriter) error {

	// Test suggestion box in a separate input field.
	if err := keyboard.AccelAction("Tab")(ctx); err != nil {
		return err
	}

	if err := TestJapaneseCandidatesBox(ctx, ui, uda, keyboard); err != nil {
		return err
	}

	return nil
}

// TestJapaneseCandidatesBoxInEditor is a wrapper around TestJapaneseCandidatesBox.
// It moves the cursor to a new line before entering the new test string, and removes the new test string after the test so that the original file in the editor is unchanged.
func TestJapaneseCandidatesBoxInEditor(ctx context.Context, ui *uiauto.Context, uda *uidetection.Context, keyboard *input.KeyboardEventWriter) error {

	// Test suggestion box on a new line.
	if err := keyboard.AccelAction("Enter")(ctx); err != nil {
		return err
	}

	if err := TestJapaneseCandidatesBox(ctx, ui, uda, keyboard); err != nil {
		return err
	}

	if err := uiauto.Combine("remove test string",
		keyboard.AccelAction("Shift+Up"),
		keyboard.AccelAction("Backspace"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to remove test string after verifying the suggestions box appears in the correct location")
	}
	return nil
}

// TestJapaneseCandidatesBox verifies that the Japanese suggestion box appears in the expected range.
// It assumes that a input field is already in focus, and the Japanese input method is enabled.
func TestJapaneseCandidatesBox(ctx context.Context, ui *uiauto.Context, uda *uidetection.Context, keyboard *input.KeyboardEventWriter) error {

	// This is a long, all upper case string so that it can be more easily detected by uidetection.
	const japaneseCandidatesBoxTestString = "TIMLOHISCOOL"

	testing.ContextLog(ctx, "Verifying suggestion box location for Japanese input")

	if err := keyboard.TypeAction(japaneseCandidatesBoxTestString)(ctx); err != nil {
		return err
	}

	candidatesLoc, err := ui.Location(ctx, PKCandidatesFinder)
	if err != nil {
		return err
	}
	candidatePt := candidatesLoc.TopLeft()

	if err := uiauto.Combine("commit the text to remove the candidates box",
		keyboard.AccelAction("Enter"),
		ui.WithTimeout(time.Second).WaitUntilGone(PKCandidatesFinder),
	)(ctx); err != nil {
		return err
	}

	// Find location of committed text.
	inputFinder := uidetection.Word(japaneseCandidatesBoxTestString)
	inputLoc, err := uda.Location(ctx, inputFinder)
	if err != nil {
		return err
	}
	inputBottomLeft := inputLoc.BottomLeft()
	inputBottomRight := inputLoc.BottomRight()

	if !isWithinRange(candidatePt, inputBottomLeft, inputBottomRight) {
		return errors.Wrapf(err, "failed to display candidates box near input. got: %v, want it to be in a 15px range around %v and %v", candidatePt, inputBottomLeft, inputBottomRight)
	}

	return nil
}

func isWithinRange(candidatePt, bottomLeft, bottomRight coords.Point) bool {
	// We expect the candidates box to have appeared within a 15px offset along the bottom of the committed text.
	const offset = 15

	// Check if the point lies within bottomLeft.X - offset and bottomRight.X + offset.
	if (bottomLeft.X-offset) > candidatePt.X || (bottomRight.X+offset) < candidatePt.X {
		return false
	}

	// Check if the point lies within bottomLeft.Y - offset and bottomRight.Y + offset.
	if (bottomLeft.Y-offset) > candidatePt.Y || (bottomRight.Y+offset) < candidatePt.Y {
		return false
	}
	return true
}
