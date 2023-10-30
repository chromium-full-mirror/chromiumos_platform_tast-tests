// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/data"
	fixture "go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture/appcompat"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
)

type citrixTestCase struct {
	TestName             string
	typingKeys           string
	expectedTypingResult string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardAppCompatCitrix,
		Desc:         "Checks that physical keyboard can perform typing in citrix",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:inputs_appcompat_citrix_perbuild"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.CitrixNotepad,
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

func PhysicalKeyboardAppCompatCitrix(ctx context.Context, s *testing.State) {
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
			util.ClearTextFieldViaClickingBackspace(kb, data.LongestInputLength),
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
