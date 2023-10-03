// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardJapaneseTyping,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that Japanese physical keyboard works",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:input-tools-upstream", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-f5823415-b90a-4ff8-aad2-2a362980afab",
			},
		},
		Params: []testing.Param{
			{
				Name:             "us",
				Fixture:          fixture.ClamshellNonVKWithAltClickAndSixPackCustomization,
				Val:              ime.JapaneseWithUSKeyboard,
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.JapaneseWithUSKeyboard}),
			},
			{
				Name:             "jp",
				Fixture:          fixture.ClamshellNonVKWithAltClickAndSixPackCustomization,
				Val:              ime.Japanese,
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.Japanese}),
			},
			{
				Name:              "us_lacros",
				Fixture:           fixture.LacrosClamshellNonVKWithAltClickAndSixPackCustomization,
				Val:               ime.JapaneseWithUSKeyboard,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.JapaneseWithUSKeyboard}),
			},
			{
				Name:              "jp_lacros",
				Fixture:           fixture.LacrosClamshellNonVKWithAltClickAndSixPackCustomization,
				Val:               ime.Japanese,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.Japanese}),
			},
		},
	})
}

// validateInputFieldFromNthCandidate returns an action that gets the candidate at the specified position and checks if the input field has the same value.
func validateInputFieldFromNthCandidate(its *testserver.InputsTestServer, tconn *chrome.TestConn, inputField testserver.InputField, n int) uiauto.Action {
	return util.GetNthCandidateTextAndThen(tconn, n, func(text string) uiauto.Action {
		return util.WaitForFieldTextToBe(tconn, inputField.Finder(), text)
	})
}

func PhysicalKeyboardJapaneseTyping(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Add IME for testing.
	im := s.Param().(ime.InputMethod)

	s.Log("Set current input method to: ", im)
	if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %v: %v: ", im, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, im.Name)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputField := testserver.TextAreaInputField

	// Focus on the input field and wait for a small duration.
	// This is needed as the Japanese IME has a bug where typing immediately after
	// a new focus will leave the first character unconverted.
	if err := its.ClickFieldAndWaitForActive(inputField)(ctx); err != nil {
		s.Fatal("Failed to wait for input field to activate: ", err)
	}
	//GoBigSleepLint: Remove this once the bug (b/191213378) is fixed.
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	ui := uiauto.New(tconn)

	subtests := []struct {
		name     string
		scenario string
		action   uiauto.Action
	}{
		// Type and check that the text field has the correct Hiragana.
		{
			name:     "TypeRomajiShowsHiragana",
			scenario: "Type Romaji and check correct Hiragana",
			action:   its.ValidateInputOnField(inputField, kb.TypeAction("nihongo"), "にほんご"),
		},
		// Type and edit the composition with various keys.
		// TODO(b/289738600): Add test for Ctrl+Space and Ctrl+Shift+Space once they work.
		{
			name:     "EditComposition",
			scenario: "Type some text and edit it",
			action: uiauto.Combine("type some text and edit it",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				// Use various shortcuts to move the cursor left character by character.
				kb.AccelAction("Left"),
				kb.AccelAction("Ctrl+k"),
				kb.AccelAction("Ctrl+s"),
				kb.AccelAction("Shift+Left"),
				// Cursor should be at the beginning of the text.
				// Insert 'aa' at the beginning.
				kb.TypeAction("aa"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ああにほんご"),
				// Use various shortcuts to move the cursor right character by character.
				kb.AccelAction("Right"),
				kb.AccelAction("Ctrl+d"),
				kb.AccelAction("Ctrl+l"),
				kb.AccelAction("Shift+Right"),
				// Cursor should be at the end of the text.
				// Insert 'aa' at the end.
				kb.TypeAction("aa"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ああにほんごああ"),
				// Move to the beginning of the text and delete the first character.
				kb.AccelAction("Ctrl+a"),
				kb.AccelAction("Ctrl+g"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "あにほんごああ"),
				// Move to the end of the text and delete the last character.
				kb.AccelAction("Ctrl+x"),
				kb.AccelAction("Ctrl+h"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "あにほんごあ"),
				// Move to the beginning of the text and delete the first character.
				kb.AccelAction("Ctrl+e"),
				kb.AccelAction("Search+Backspace"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "にほんごあ"),
				// Move to the end of the text and delete the last character.
				kb.AccelAction("Ctrl+f"),
				kb.AccelAction("Backspace"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "にほんご"),
				// Move to the beginning of the text and delete the first character.
				kb.AccelAction("Ctrl+Left"),
				kb.AccelAction("Search+Backspace"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ほんご"),
				// Move to the end of the text and delete the last character.
				kb.AccelAction("Ctrl+Right"),
				kb.AccelAction("Backspace"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ほん"),
				// Move to the beginning of the text, move right, and replace.
				kb.AccelAction("Ctrl+Up"),
				kb.AccelAction("Right"),
				kb.AccelAction("Backspace"),
				kb.TypeAction("a"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "あん"),
				// Move to the end of the text, move left, and replace .
				kb.AccelAction("Ctrl+Down"),
				kb.AccelAction("Left"),
				kb.AccelAction("Search+Backspace"),
				kb.TypeAction("a"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ああ"),
			),
		},
		// Type and press Tab/Shift+Tab to select different candidates.
		// The text field should show the selected candidate.
		{
			name:     "TabCyclesThroughCandidates",
			scenario: "Type and press Tab/Shift+Tab to select candidate",
			action: uiauto.Combine("Use Tab key to Cycle through candidates",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				uiauto.Repeat(3, kb.AccelAction("Tab")),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 2),
				kb.AccelAction("Shift+Tab"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 1),
			),
		},
		// Type and press arrow keys to select different candidates.
		// The text field should show the selected candidate.
		{
			name:     "ArrowKeysCycleThroughCandidates",
			scenario: "Use arrow keys to cycle through candidates",
			action: uiauto.Combine("cycle through candidates with arrow keys",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				uiauto.Repeat(3, kb.AccelAction("Down")),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 2),
				kb.AccelAction("Up"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 1),
			),
		},
		// Type and press Tab/Arrow keys to go through multiple pages of candidates.
		// The text field should show the selected candidate.
		{
			name:     "TabAndArrowKeysCyclesThroughPages",
			scenario: "Use Tab/Arrow keys to flip pages",
			action: uiauto.Combine("cycle through pages with tab and arrow keys",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				// The Japanese IME shows a max of 9 candidates per page.
				uiauto.Repeat(10, kb.AccelAction("Tab")),
				uiauto.Repeat(5, kb.AccelAction("Down")),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 5),
			),
		},
		// Type and press a number key to select the candidate with that number.
		// The text field should show the selected candidate.
		{
			name:     "NumberKeySelectsCandidate",
			scenario: "Use number key to select the candidate",
			action: uiauto.Combine("bring up candidates and select with number key",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				kb.AccelAction("Tab"),
				uiauto.Repeat(5, kb.AccelAction("Tab")),
				kb.TypeAction("3"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 2),
			),
		},
		// Type and press space, which should select the first conversion candidate and hide the candidates window.
		{
			name:     "SpaceSelectsTopConversionCandidate",
			scenario: "Use SPACE key to select the first conversion candidate",
			action: uiauto.Combine("bring up the conversion candidates window",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				// Pop up the conversion candidates window to find the top conversion candidate.
				kb.AccelAction("Space"),
				kb.AccelAction("Space"),
				util.GetNthCandidateTextAndThen(tconn, 0, func(text string) uiauto.Action {
					return uiauto.Combine("retype and press space to select default candidate",
						its.ClearThenClickFieldAndWaitForActive(inputField),
						kb.TypeAction("nihongo"),
						kb.AccelAction("Space"),
						ui.WaitUntilGone(util.PKCandidatesFinder),
						util.WaitForFieldTextToBe(tconn, inputField.Finder(), text),
					)
				}),
			),
		},
		// Type and use shortcuts to go through different conversion candidates.
		// The text field should show the selected candidate.
		{
			name:     "ShortcutsCycleThroughConversionCandidates",
			scenario: "Type and use shortcuts to go through different conversion candidates",
			action: uiauto.Combine("type some text",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				uiauto.Repeat(5, kb.AccelAction("Space")),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 4),
				kb.AccelAction("Ctrl+e"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 3),
				kb.AccelAction("Ctrl+up"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 2),
				kb.AccelAction("Ctrl+x"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 3),
				kb.AccelAction("Down"),
				validateInputFieldFromNthCandidate(its, tconn, inputField, 4),
			),
		},
		// Type and Tab several times to select a candidate.
		// Press Enter, which should submit the selected candidate and hide the candidates window.
		{
			name:     "EnterSubmitsCandidate",
			scenario: "Type and Tab several times to select a candidate",
			action: uiauto.Combine("type some text",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				uiauto.Repeat(3, kb.AccelAction("Tab")),
				util.GetNthCandidateTextAndThen(tconn, 2, func(text string) uiauto.Action {
					return uiauto.Combine("press enter and verify text",
						kb.AccelAction("Enter"),
						ui.WaitUntilGone(util.PKCandidatesFinder),
						util.WaitForFieldTextToBe(tconn, inputField.Finder(), text),
					)
				}),
			),
		},
		// Use shortcuts to convert the output script (e.g. Hiragana, Katakana).
		{
			name:     "ConvertScriptShortcuts",
			scenario: "Use shortcuts to convert the output script",
			action: uiauto.Combine("type some text and press shortcuts",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("nihongo"),
				// Convert to Hiragana
				kb.AccelAction("Ctrl+u"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "にほんご"),
				// Convert to Half Width Katakana
				kb.AccelAction("Ctrl+o"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ﾆﾎﾝｺﾞ"),
				// Convert to Katakana
				kb.AccelAction("Ctrl+i"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ニホンゴ"),
				// Convert to Latin
				kb.AccelAction("Ctrl+p"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ｎｉｈｏｎｇｏ"),
				// Convert to Half Width Latin
				kb.AccelAction("Ctrl+t"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "nihongo"),
				// Same as above, but with function keys.
				kb.AccelAction("F6"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "にほんご"),
				kb.AccelAction("F8"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ﾆﾎﾝｺﾞ"),
				kb.AccelAction("F7"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ニホンゴ"),
				kb.AccelAction("F9"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "ｎｉｈｏｎｇｏ"),
				kb.AccelAction("F10"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "nihongo"),
			),
		},
		// Type a complex phrase and convert the segments individually.
		// Although other tests do not rely on specific candidates, this one unavoidably relies on "tenkigaii" converting to 天気がいい.
		// If that's not the case, it's highly likely there's a bug in the input method.
		{
			name:     "ConversionSegments",
			scenario: "Use shortcuts to convert segments of a phrase",
			action: uiauto.Combine("type some text and press shortcuts",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				// Create two segments: 天気が|いい
				kb.TypeAction("tenkigaii "),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "天気がいい"),
				// Shrink the first segment to create 3 segments: 天気|が|いい
				kb.AccelAction("Shift+Left"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "天気がいい"),
				// Move right to the 2nd segment (が) and convert it to Half Width Latin.
				kb.AccelAction("Right"),
				kb.AccelAction("Ctrl+t"),
				// Move right to the 3rd segment (いい) and convert it to Hiragana.
				kb.AccelAction("Ctrl+d"),
				kb.AccelAction("Ctrl+u"),
				// Jump back to 1st segment (天気) and convert it to Katakana.
				kb.AccelAction("Ctrl+a"),
				kb.AccelAction("Ctrl+i"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "テンキgaいい"),
				// Jump to last segment and convert it to Katakana.
				kb.AccelAction("Ctrl+f"),
				kb.AccelAction("Ctrl+i"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "テンキgaイイ"),
				// Jump to first segment and convert it to Half Width Latin.
				kb.AccelAction("Ctrl+Left"),
				kb.AccelAction("Ctrl+t"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "tenkigaイイ"),
				// Jump to last segment and convert it to Hiragana.
				kb.AccelAction("Ctrl+Right"),
				kb.AccelAction("Ctrl+u"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "tenkigaいい"),
				// Move left to the middle segment and convert it to Hiragana.
				kb.AccelAction("Left"),
				kb.AccelAction("Ctrl+u"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "tenkiがいい"),
				// Move left to the first segment and convert it to Hiragana.
				kb.AccelAction("Ctrl+s"),
				kb.AccelAction("Ctrl+u"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "てんきがいい"),
				// Expand the first segment so that there's only two segments: 天気が|いい
				// It should automatically convert back to the original Kanji result.
				kb.AccelAction("Shift+Right"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "天気がいい"),
				// Commit only the first segment, leaving only いい in composition.
				kb.AccelAction("Ctrl+Down"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "天気がいい"),
				// Shrink the segment so there's two segments: い|い
				// Then convert it to Half Width Latin
				kb.AccelAction("Shift+Left"),
				kb.AccelAction("Ctrl+t"),
				kb.AccelAction("Ctrl+n"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "天気がiい"),
				// Convert the final segment (い) to Katakana.
				kb.AccelAction("Ctrl+i"),
				util.WaitForFieldTextToBe(tconn, inputField.Finder(), "天気がiイ"),
			),
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(subtest.name))

			if err := uiauto.UserAction(
				"Japanese PK input",
				subtest.action,
				uc, &useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: subtest.scenario,
						useractions.AttributeFeature:      useractions.FeaturePKTyping,
						useractions.AttributeInputField:   string(inputField),
					},
				},
			)(ctx); err != nil {
				s.Fatalf("Failed to validate keys input in %s: %v", inputField, err)
			}
		})
	}
}
