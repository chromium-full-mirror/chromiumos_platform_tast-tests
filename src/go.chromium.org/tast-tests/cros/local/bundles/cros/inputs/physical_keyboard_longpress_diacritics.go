// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testrunner"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardLongpressDiacritics,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks diacritics on long-press with physical keyboard typing",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		Timeout:      2 * time.Minute,
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUS},
			[]string{
				"screenplay-83bc7d24-4370-4c51-92a0-b63d961c483d",
				"screenplay-3cd04977-4eaf-403a-a4da-4d59e4069338",
				"screenplay-739015bc-416f-4da4-8703-410b9f7f1926",
				"screenplay-26c40157-cf25-4cd7-a315-dbba94391197",
			}),
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVKWithDiacriticsOnPKLongpress,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVKWithDiacriticsOnPKLongpress,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
		},
	})
}

func PhysicalKeyboardLongpressDiacritics(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// PK longpress diacritics only works in English(US).
	inputMethod := ime.EnglishUS

	if err := inputMethod.Activate(tconn)(ctx); err != nil {
		s.Fatal("Failed to set IME: ", err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	const inputField = testserver.TextInputField

	const (
		longpressKeyChar = "a"
		diacritic        = "à"
	)

	candidateWindowFinder := nodewith.HasClass("SuggestionWindowView").Role(role.Window)
	suggestionCharFinder := nodewith.Name(diacritic).Ancestor(candidateWindowFinder).First()
	ui := uiauto.New(tconn)

	testCases := []struct {
		name     string
		scenario string
		// The action occur while suggestion window is open and should result in the window being closed.
		actions      uiauto.Action
		expectedText string
	}{
		{
			name:         "left_click",
			scenario:     "PK longpress and left click to insert diacritics",
			actions:      ui.LeftClick(suggestionCharFinder),
			expectedText: diacritic,
		},
		{
			name:     "right_arrow_enter",
			scenario: "PK longpress and arrow key then enter to insert diacritics",
			actions: uiauto.Combine("right arrow then enter",
				kb.AccelAction("Right"),
				kb.AccelAction("Enter"),
			),
			expectedText: diacritic,
		},
		{
			name:         "number_key",
			scenario:     "PK longpress and number key to insert diacritics",
			actions:      kb.AccelAction("1"),
			expectedText: diacritic,
		},
		{
			name:         "esc_to_dismiss",
			scenario:     "PK longpress and esc to dismiss",
			actions:      kb.AccelAction("Esc"),
			expectedText: longpressKeyChar,
		},
	}

	for _, testcase := range testCases {
		testrunner.RunSubTest(ctx, s, cr, testcase.name, uiauto.UserAction(testcase.scenario,
			uiauto.Combine(testcase.scenario,
				its.Clear(inputField),
				its.ClickFieldAndWaitForActive(inputField),
				// Simulate a held down key until window appears.
				kb.AccelPressAction(longpressKeyChar),
				ui.WaitUntilExists(candidateWindowFinder),
				kb.AccelReleaseAction(longpressKeyChar),
				testcase.actions,
				ui.WaitUntilGone(candidateWindowFinder),
				its.ValidateResult(inputField, testcase.expectedText),
			),
			uc,
			&useractions.UserActionCfg{
				Attributes: map[string]string{
					useractions.AttributeTestScenario: testcase.scenario,
					useractions.AttributeFeature:      useractions.FeatureLongpressDiacritics,
				},
			},
		))
	}
}
