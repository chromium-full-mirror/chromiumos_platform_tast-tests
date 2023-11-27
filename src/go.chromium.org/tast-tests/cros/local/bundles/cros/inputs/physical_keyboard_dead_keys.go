// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
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

// deadKeysTestCase struct encapsulates parameters for each Dead Keys test.
type pkDeadKeysTestCase struct {
	inputMethod          ime.InputMethod
	typingKeys           string
	expectedTypingResult string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardDeadKeys,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that dead keys on the physical keyboard work",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-bd19bdd3-ae1e-4361-8683-fe2bdac03af0",
			},
		},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      5 * time.Minute,

		Params: []testing.Param{
			{
				Name:    "french",
				Fixture: fixture.ClamshellNonVK,
				Val: pkDeadKeysTestCase{
					inputMethod:          ime.FrenchFrance,
					typingKeys:           "[e",
					expectedTypingResult: "ê",
				},
				ExtraAttr:        []string{"group:input-tools-upstream", "group:hw_agnostic"},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			},
			{
				Name:    "us_intl_acute",
				Fixture: fixture.ClamshellNonVK,
				Val: pkDeadKeysTestCase{
					inputMethod:          ime.EnglishUSWithInternationalKeyboard,
					typingKeys:           "'a",
					expectedTypingResult: "á",
				},
				ExtraAttr:        []string{"group:input-tools-upstream", "group:hw_agnostic"},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard}),
			},
			{
				Name:    "us_intl_double",
				Fixture: fixture.ClamshellNonVK,
				Val: pkDeadKeysTestCase{
					inputMethod:          ime.EnglishUSWithInternationalKeyboard,
					typingKeys:           "''",
					expectedTypingResult: "´",
				},
				ExtraAttr:        []string{"group:input-tools-upstream", "group:hw_agnostic"},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard}),
			},
			{
				Name:    "french_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkDeadKeysTestCase{
					inputMethod:          ime.FrenchFrance,
					typingKeys:           "[e",
					expectedTypingResult: "ê",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			},
			{
				Name:    "us_intl_acute_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkDeadKeysTestCase{
					inputMethod:          ime.EnglishUSWithInternationalKeyboard,
					typingKeys:           "'a",
					expectedTypingResult: "á",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard}),
			},
			{
				Name:    "us_intl_double_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkDeadKeysTestCase{
					inputMethod:          ime.EnglishUSWithInternationalKeyboard,
					typingKeys:           "''",
					expectedTypingResult: "´",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard}),
			},
		},
	})
}

func PhysicalKeyboardDeadKeys(ctx context.Context, s *testing.State) {
	testCase := s.Param().(pkDeadKeysTestCase)

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputMethod := testCase.inputMethod
	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %s: %v: ", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	inputField := testserver.TextAreaInputField

	validateAction := uiauto.Combine("validate dead keys typing",
		its.ClickFieldAndWaitForActive(inputField),
		kb.TypeAction(testCase.typingKeys),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.expectedTypingResult),
	)

	if err := uiauto.UserAction(
		"PK dead keys input",
		validateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureDeadKeys,
				useractions.AttributeTestScenario: fmt.Sprintf(`type %q to get %q`, testCase.typingKeys, testCase.expectedTypingResult),
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to verify dead keys input: ", err)
	}
}
