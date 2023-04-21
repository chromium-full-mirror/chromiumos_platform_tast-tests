// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/local/bundles/cros/inputs/data"
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
const longestInputLength = 10

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardCitrixTyping,
		Desc:         "Checks that physical keyboard can perform typing in citrix",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:244259740",
		Attr:         []string{"group:mainline", "group:input-tools", "informational", "group:criticalstaging"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
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

	inputMethod := s.Param().(ime.InputMethod)

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %s: %v: ", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	for _, subtest := range data.AppCompatPhysicalKeyboardTestCases[inputMethod] {
		validateAction := uiauto.Combine("validate dead keys typing",
			clearText(kb, longestInputLength),
			kb.TypeSequenceAction(subtest.LocationKeySeq),
			uidetector.WithScreenshotResizing().WaitUntilExists(uidetection.TextBlock(strings.Split(subtest.ExpectedText, " "))),
		)

		s.Run(ctx, subtest.Description, func(ctx context.Context, s *testing.State) {
			if err := uiauto.UserAction(
				subtest.Description,
				validateAction,
				uc, &useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: subtest.Description,
						useractions.AttributeFeature:      useractions.FeaturePKTyping,
					},
				},
			)(ctx); err != nil {
				s.Fatalf("Failed to validate %s typing in test %s: %v", inputMethod, subtest.Description, err)
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
