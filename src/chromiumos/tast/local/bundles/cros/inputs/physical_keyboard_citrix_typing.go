// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	fixture "chromiumos/tast/local/bundles/cros/inputs/fixture/appcompat"
	"chromiumos/tast/local/bundles/cros/inputs/pre"
	"chromiumos/tast/local/bundles/cros/inputs/util"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/useractions"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type citrixTestCase struct {
	TestName             string
	typingKeys           string
	expectedTypingResult string
}

// longest input length, use this var to clean the env between sub tests.
const longestInputLength = 7

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardCitrixTyping,
		Desc:         "Checks that physical keyboard can perform typing in citrix",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:244259740",
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.CitrixNotepad,
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Params: []testing.Param{
			{
				Name:             "french",
				Val:              ime.FrenchFrance,
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			},
			{
				Name:             "us_intl_acute",
				Val:              ime.EnglishUSWithInternationalKeyboard,
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard}),
			},
		},
	})
}

func PhysicalKeyboardCitrixTyping(ctx context.Context, s *testing.State) {
	uidetector := s.FixtValue().(fixture.CitrixNotepadFixtData).UIDetector
	uc := s.FixtValue().(fixture.CitrixNotepadFixtData).UserContext
	kb := s.FixtValue().(fixture.CitrixNotepadFixtData).Keyboard

	languageTests := map[ime.InputMethod][]citrixTestCase{
		ime.FrenchFrance: {
			{
				TestName:             "dead key",
				typingKeys:           "t[est",
				expectedTypingResult: "têst",
			},
			{
				TestName:             "number key 0 to 2",
				typingKeys:           "h0h1h2",
				expectedTypingResult: "hàh&hé",
			},
		},
		ime.EnglishUSWithInternationalKeyboard: {
			{
				TestName:             "dead key",
				typingKeys:           "'abc",
				expectedTypingResult: "ábc",
			},
		},
	}

	inputMethod := s.Param().(ime.InputMethod)

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %s: %v: ", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	for _, subtest := range languageTests[inputMethod] {
		validateAction := uiauto.Combine("validate dead keys typing",
			clearText(kb, longestInputLength),
			kb.TypeAction(subtest.typingKeys),
			uidetector.WithScreenshotResizing().WaitUntilExists(uidetection.TextBlock(strings.Split(subtest.expectedTypingResult, " "))),
		)

		s.Run(ctx, subtest.TestName, func(ctx context.Context, s *testing.State) {
			if err := uiauto.UserAction(
				subtest.TestName,
				validateAction,
				uc, &useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: subtest.TestName,
						useractions.AttributeFeature:      useractions.FeaturePKTyping,
					},
				},
			)(ctx); err != nil {
				s.Fatalf("Failed to validate %s typing in test %s: %v", inputMethod, subtest.TestName, err)
			}
		})
	}

}

func clearText(kb *input.KeyboardEventWriter, times int) action.Action {
	return kb.TypeSequenceAction(makeUpKeySequence(times, "Backspace"))
}

func makeUpKeySequence(times int, key string) []string {
	var keySequence = []string{}
	for i := 1; i <= times; i++ {
		keySequence = append(keySequence, key)
	}

	return keySequence
}
