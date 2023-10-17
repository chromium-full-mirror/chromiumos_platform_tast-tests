// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardMultiwordSuggestion,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks on device multiword suggestions with physical keyboard typing",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SearchFlags:  util.SearchFlagsWithIMEAndScreenPlay([]ime.InputMethod{ime.EnglishUS}, []string{"screenplay-d72ccf01-4e0f-451e-83fd-040151fc9722"}),
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal", "ondevice_text_suggestions"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				// TODO(b/299975550): Diagnose flakiness and remove "informational".
				Fixture:           fixture.ClamshellNonVKWithMultiwordSuggest,
				ExtraAttr:         []string{"group:input-tools-upstream", "informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVKWithMultiwordSuggest,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVKWithMultiwordSuggest,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
		},
	})
}

func PhysicalKeyboardMultiwordSuggestion(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// PK multiword suggestion only works in English(US).
	inputMethod := ime.EnglishUS

	// Activate function checks the current IME. It does nothing if the given input method is already in-use.
	// It is called here just in case IME has been changed in last test.
	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatal("Failed to set IME: ", err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	inputField := testserver.TextAreaInputField
	suggestionWindowFinder := nodewith.HasClass("SuggestionWindowView").Role(role.Window)
	ui := uiauto.New(tconn)

	// TODO(b/224628222): Expecting an ML candidate to remain the same in
	//   tests can be somewhat flakey in future runs of the test. Update
	//   these tests to capture the candidate shown on screen and validate
	//   against that candidate.
	subtests := []struct {
		name     string
		scenario string
		errStr   string
		action   uiauto.Action
	}{
		{
			// Trigger suggestion "look at" and insert into
			// textfield with tab.
			name:     "AcceptSuggestionWithTab",
			scenario: "verify suggestion appears and accepted with tab key",
			errStr:   "Failed to accept suggestion: %v",
			action: uiauto.Combine("accept multiword suggestion with tab",
				keyboard.TypeAction("please take a lo"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "please take a lo"),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.AccelAction("Tab"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "please take a look at"),
			),
		},
		{
			// Trigger suggestion "look at" and insert into
			// textfield with down + enter key.
			name:     "AcceptSuggestionWithDownAndEnter",
			scenario: "verify suggestion appears and accepted with down and enter key",
			errStr:   "Failed to accept suggestion: %v",
			action: uiauto.Combine("accept multiword suggestion with down and enter",
				keyboard.TypeAction("please take a lo"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "please take a lo"),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.AccelAction("Down"),
				keyboard.AccelAction("Enter"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "please take a look at"),
			),
		},
		{
			// Trigger suggestion "let me know" and dismiss with
			// multiple white space at the end of the text.
			name:     "SuggestionShouldAppearOnlyAtEndOfText",
			scenario: "verify suggestion dismissed with multiple whitespace",
			errStr:   "Failed to dismiss suggestion with whitespace: %v",
			action: uiauto.Combine("dismiss multiword suggestion with multiple whitespace",
				keyboard.TypeAction("let me "),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "let me "),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.TypeAction(" "),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "let me  "),
				ui.WaitUntilGone(suggestionWindowFinder),
			),
		},
		{
			// Trigger suggestion "sounds good", partially type
			// suggestion, and dismiss suggestion by deleting text
			// beyond suggestion trigger point.
			name:     "SuggestionTrackedAndDismissedWithTextUpdates",
			scenario: "track typing in suggestion and dismiss when deleting past trigger point",
			errStr:   "Failed to dismiss suggestion: %v",
			action: uiauto.Combine("dismiss multiword suggestion by deleting past trigger point",
				keyboard.TypeAction("ok so"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "ok so"),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.TypeAction("unds g"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "ok sounds g"),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.AccelAction("Backspace"), // "ok sounds "
				keyboard.AccelAction("Backspace"), // "ok sounds"
				keyboard.AccelAction("Backspace"), // "ok sound"
				keyboard.AccelAction("Backspace"), // "ok soun"
				keyboard.AccelAction("Backspace"), // "ok sou"
				keyboard.AccelAction("Backspace"), // "ok so"
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.AccelAction("Backspace"), // "ok s"
				ui.WaitUntilGone(suggestionWindowFinder),
			),
		},
		{
			// Suggestions should handle newlines gracefully.
			name:     "SuggestionHandlesNewline",
			scenario: "suggestions handles newline gracefully",
			errStr:   "Failed to accept suggestion: %v",
			action: uiauto.Combine("suggestion handles newline gracefully",
				keyboard.TypeAction("I am look"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "I am look"),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.AccelAction("Enter"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "I am look\n"),
				ui.WaitUntilGone(suggestionWindowFinder),
				keyboard.AccelAction("Backspace"),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "I am look"),
				ui.WaitUntilExists(suggestionWindowFinder),
				keyboard.AccelAction("Tab"),
				ui.WaitUntilGone(suggestionWindowFinder),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), "I am looking for"),
			),
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(subtest.name))

			if err := uiauto.UserAction(
				"Multiword suggestion",
				uiauto.Combine("...",
					its.Clear(inputField),
					its.ClickFieldAndWaitForActive(inputField),
					subtest.action,
				),
				uc, &useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: subtest.scenario,
						useractions.AttributeInputField:   string(inputField),
						useractions.AttributeFeature:      useractions.FeatureMultiwordSuggestion,
					},
				},
			)(ctx); err != nil {
				s.Fatalf(subtest.errStr, err)
			}
		})
	}
}
