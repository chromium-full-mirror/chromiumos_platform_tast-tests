// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardJapaneseSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the Japanese input settings (excluding user dictionaries) work for physical keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.JapaneseWithUSKeyboard}),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "us",
				Fixture:           fixture.ClamshellNonVKRestart,
				Val:               ime.JapaneseWithUSKeyboard,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
			{
				Name:              "jp",
				Fixture:           fixture.ClamshellNonVKRestart,
				Val:               ime.Japanese,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
		},
	})
}

// getBracketSlashKeys returns the key sequence to type "[]/", depending on whether it's Japanese or US layout.
func getBracketSlashKeys(im ime.InputMethod) string {
	if im == ime.Japanese {
		return "]\\/"
	}
	return "[]/"
}

func PhysicalKeyboardJapaneseSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	// Use text input field as testing target.
	inputField := testserver.TextInputField

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

	ui := uiauto.New(tconn)

	// The subtests runs in sequence, so any changed settings will persist to the next subtest.
	subtests := []struct {
		// Unique name for the subtest.
		name string

		// The useraction feature that the subtest tests.
		feature string

		// The uiauto.Action to run inside the Japanese settings page.
		settingAction uiauto.Action

		// The uiauto.Action to run in the input field to validate the setting has been applied correctly.
		validationAction uiauto.Action
	}{
		{
			name:             "InputModeKana",
			feature:          useractions.FeatureJapaneseInputMode,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseInputMode, "Kana"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("tu"), "かな"),
		},
		{
			name:             "InputModeRomaji",
			feature:          useractions.FeatureJapaneseInputMode,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseInputMode, "Romaji"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("romaji"), "ろまじ"),
		},
		{
			name:             "PunctuationStyleCommaPeriod",
			feature:          useractions.FeatureJapanesePunctuationStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapanesePunctuationStyle, "，．"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(",."), "，．"),
		},
		{
			name:             "PunctuationStyleKutenPeriod",
			feature:          useractions.FeatureJapanesePunctuationStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapanesePunctuationStyle, "、．"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(",."), "、．"),
		},
		{
			name:             "PunctuationStyleCommaTouten",
			feature:          useractions.FeatureJapanesePunctuationStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapanesePunctuationStyle, "，。"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(",."), "，。"),
		},
		{
			name:             "PunctuationStyleKutenTouten",
			feature:          useractions.FeatureJapanesePunctuationStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapanesePunctuationStyle, "、。"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(",."), "、。"),
		},
		{
			name:             "SymbolStyleSquareBracketSlash",
			feature:          useractions.FeatureJapaneseSymbolStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSymbolStyle, "［］／"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(getBracketSlashKeys(im)), "［］／"),
		},
		{
			name:             "SymbolStyleCornerBracketSlash",
			feature:          useractions.FeatureJapaneseSymbolStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSymbolStyle, "「」／"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(getBracketSlashKeys(im)), "「」／"),
		},
		{
			name:             "SymbolStyleSquareBracketMiddleDot",
			feature:          useractions.FeatureJapaneseSymbolStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSymbolStyle, "［］・"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(getBracketSlashKeys(im)), "［］・"),
		},
		{
			name:             "SymbolStyleCornerBracketMiddleDot",
			feature:          useractions.FeatureJapaneseSymbolStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSymbolStyle, "「」・"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(getBracketSlashKeys(im)), "「」・"),
		},
		{
			name:             "SpaceInputStyleFullwidth",
			feature:          useractions.FeatureJapaneseSpaceInputStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSpaceInputStyle, "Fullwidth"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(" "), "　"),
		},
		{
			name:             "SpaceInputStyleHalfwidth",
			feature:          useractions.FeatureJapaneseSpaceInputStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSpaceInputStyle, "Halfwidth"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(" "), " "),
		},
		{
			name:             "SpaceInputStyleFollowInputMode",
			feature:          useractions.FeatureJapaneseSpaceInputStyle,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSpaceInputStyle, "Follow input mode"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction(" "), "　"),
		},
		{
			name:          "SelectionShortcutAToL",
			feature:       useractions.FeatureJapaneseSelectionShortcut,
			settingAction: imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSelectionShortcut, "A -- L"),
			validationAction: uiauto.Combine("trigger candidates window",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("a  "),
				util.GetNthCandidateTextAndThen(tconn, 3, func(text string) uiauto.Action {
					return uiauto.Combine("select 4th candidate", kb.TypeAction("f"), its.ValidateResult(inputField, text))
				}),
			),
		},
		{
			name:          "SelectionShortcutNone",
			feature:       useractions.FeatureJapaneseSelectionShortcut,
			settingAction: imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSelectionShortcut, "No shortcut"),
			validationAction: uiauto.Combine("trigger candidates window",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("a  "),
				util.GetNthCandidateTextAndThen(tconn, 1, func(text string) uiauto.Action {
					return uiauto.Combine("append to composition", kb.TypeAction("a1"), its.ValidateResult(inputField, text+"あ１"))
				}),
			),
		},
		{
			name:          "SelectionShortcut1To9",
			feature:       useractions.FeatureJapaneseSelectionShortcut,
			settingAction: imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseSelectionShortcut, "1 -- 9"),
			validationAction: uiauto.Combine("trigger candidates window",
				its.ClearThenClickFieldAndWaitForActive(inputField),
				kb.TypeAction("a  "),
				util.GetNthCandidateTextAndThen(tconn, 5, func(text string) uiauto.Action {
					return uiauto.Combine("select 6th candidate", kb.TypeAction("6"), its.ValidateResult(inputField, text))
				}),
			),
		},
		{
			name:          "KeymapStyleATOK",
			feature:       useractions.FeatureJapaneseKeymapStyle,
			settingAction: imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseKeymapStyle, "ATOK"),
			// Ctrl+n is "commit" only in ATOK.
			validationAction: its.ValidateInputOnField(inputField, uiauto.Combine("type and commit letter by letter", kb.TypeAction("r"), kb.AccelAction("Ctrl+n"), kb.TypeAction("i")), "ｒい"),
		},
		{
			name:          "KeymapStyleKotoeri",
			feature:       useractions.FeatureJapaneseKeymapStyle,
			settingAction: imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseKeymapStyle, "Kotoeri"),
			// Ctrl+; is "convert to halfwidth" only in Kotoeri.
			validationAction: its.ValidateInputOnField(inputField, uiauto.Combine("type and convert to halfwidth", kb.TypeAction("a"), kb.AccelAction("Ctrl+;")), "ｱ"),
		},
		// TODO(b/259482133): Add subtest for MS-IME keymap. Identifying if the keymap is MS-IME or ChromeOS requires special keys like Henkan and Muhenkan, which are not currently supported in Tast.
		{
			name:          "KeymapStyleChromeOS",
			feature:       useractions.FeatureJapaneseKeymapStyle,
			settingAction: imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseKeymapStyle, "Chrome OS"),
			// Ctrl+z; is "cancel composition" only in ChromeOS.
			validationAction: its.ValidateInputOnField(inputField, uiauto.Combine("type and cancel composition", kb.TypeAction("a"), kb.AccelAction("Ctrl+z")), ""),
		},
		{
			name:             "AutomaticallySwitchToHalfwidthEnabled",
			feature:          useractions.FeatureJapaneseAutomaticallySwitchToHalfwidth,
			settingAction:    imesettings.SetJapaneseCheckbox(ui, imesettings.JapaneseAutomaticallySwitchToHalfwidth, checked.False),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("http"), "ｈっｔｐ"),
		},
		{
			name:             "AutomaticallySwitchToHalfwidthDisabled",
			feature:          useractions.FeatureJapaneseAutomaticallySwitchToHalfwidth,
			settingAction:    imesettings.SetJapaneseCheckbox(ui, imesettings.JapaneseAutomaticallySwitchToHalfwidth, checked.True),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("http"), "http"),
		},
		{
			name:             "ShiftKeyModeSwitchKatakana",
			feature:          useractions.FeatureJapaneseShiftKeyModeSwitch,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseShiftKeyModeSwitch, "Katakana"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("A"), "ア"),
		},
		{
			name:             "ShiftKeyModeSwitchOff",
			feature:          useractions.FeatureJapaneseShiftKeyModeSwitch,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseShiftKeyModeSwitch, "Off"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("A"), "あ"),
		},
		{
			name:             "ShiftKeyModeSwitchAlphanumeric",
			feature:          useractions.FeatureJapaneseShiftKeyModeSwitch,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseShiftKeyModeSwitch, "Alphanumeric"),
			validationAction: its.ValidateInputOnField(inputField, kb.TypeAction("A"), "A"),
		},
		{
			name:             "UseInputHistoryDisabled",
			feature:          useractions.FeatureJapaneseUseInputHistory,
			settingAction:    imesettings.SetJapaneseCheckbox(ui, imesettings.JapaneseUseInputHistory, checked.False),
			validationAction: uiauto.Combine("type and check suggestions do not include previous history", its.ClearThenClickFieldAndWaitForActive(inputField), kb.TypeAction("TAST123\nTAST"), ui.EnsureGoneFor(util.PKCandidatesFinder.Name("TAST123"), time.Second)),
		},
		{
			name:             "UseInputHistoryEnabled",
			feature:          useractions.FeatureJapaneseUseInputHistory,
			settingAction:    imesettings.SetJapaneseCheckbox(ui, imesettings.JapaneseUseInputHistory, checked.True),
			validationAction: uiauto.Combine("type and check suggestions include previous history", its.ClearThenClickFieldAndWaitForActive(inputField), kb.TypeAction("TAST123\nTAST"), ui.WaitUntilExists(util.PKCandidatesFinder.Name("TAST123"))),
		},
		{
			name:             "UseSystemDictionaryDisabled",
			feature:          useractions.FeatureJapaneseUseSystemDictionary,
			settingAction:    imesettings.SetJapaneseCheckbox(ui, imesettings.JapaneseUseSystemDictionary, checked.False),
			validationAction: uiauto.Combine("type check suggestions do not include system dictionary", its.ClearThenClickFieldAndWaitForActive(inputField), kb.TypeAction("genki"), ui.EnsureGoneFor(util.PKCandidatesFinder, time.Second)),
		},
		{
			name:             "UseSystemDictionaryEnabled",
			feature:          useractions.FeatureJapaneseUseSystemDictionary,
			settingAction:    imesettings.SetJapaneseCheckbox(ui, imesettings.JapaneseUseSystemDictionary, checked.True),
			validationAction: uiauto.Combine("type and check suggestions include system dictionary", its.ClearThenClickFieldAndWaitForActive(inputField), kb.TypeAction("genki"), ui.WaitUntilExists(util.PKCandidatesFinder.First())),
		},
		{
			name:             "NumberOfSuggestions9",
			feature:          useractions.FeatureJapaneseNumberOfSuggestions,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseNumberOfSuggestions, "9"),
			validationAction: uiauto.Combine("type and check number of candidates", kb.TypeAction("genki"), util.WaitForNCandidates(tconn, 9)),
		},
		{
			name:             "NumberOfSuggestions5",
			feature:          useractions.FeatureJapaneseNumberOfSuggestions,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseNumberOfSuggestions, "5"),
			validationAction: uiauto.Combine("type and check number of candidates", kb.TypeAction("genki"), util.WaitForNCandidates(tconn, 5)),
		},
		{
			name:             "NumberOfSuggestions1",
			feature:          useractions.FeatureJapaneseNumberOfSuggestions,
			settingAction:    imesettings.SetJapaneseDropdown(ui, imesettings.JapaneseNumberOfSuggestions, "1"),
			validationAction: uiauto.Combine("type and check number of candidates", kb.TypeAction("genki"), util.WaitForNCandidates(tconn, 1)),
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+subtest.name)

			validateAction := uiauto.Combine("verify settings for physical keyboard",
				imesettings.SetJapaneseKeyboardSettings(uc, ui, im, subtest.settingAction), subtest.validationAction,
			)

			if err := uiauto.UserAction(
				fmt.Sprintf("Change %q setting and check that it is applied", subtest.feature),
				validateAction,
				uc,
				&useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeInputField:   string(inputField),
						useractions.AttributeTestScenario: fmt.Sprintf("Change %q setting", subtest.feature),
						useractions.AttributeFeature:      subtest.feature,
					},
				},
			)(ctx); err != nil {
				s.Fatal("Failed to verify input: ", err)
			}
		})
	}
}
