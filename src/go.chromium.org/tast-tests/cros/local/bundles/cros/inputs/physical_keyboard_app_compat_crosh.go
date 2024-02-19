// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	fixture "go.chromium.org/tast-tests/cros/local/inputs/fixture/appcompat"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardAppCompatCrosh,
		Desc:         "Checks that physical keyboard can perform typing in crosh console",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.Crosh,
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Params: []testing.Param{
			{
				Name:             "french",
				Val:              ime.FrenchFrance,
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			},
			// {
			// 	Name:             "us_intl_acute", // known issue (b:289319217)
			// 	Val:              ime.EnglishUSWithInternationalKeyboard,
			// 	ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.EnglishUSWithInternationalKeyboard}),
			// },
		},
	})
}

func PhysicalKeyboardAppCompatCrosh(ctx context.Context, s *testing.State) {
	uc := s.FixtValue().(fixture.CroshFixtData).UserContext
	kb := s.FixtValue().(fixture.CroshFixtData).Keyboard
	ud := s.FixtValue().(fixture.CroshFixtData).UIDetector

	inputMethod := s.Param().(ime.InputMethod)

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %s: %v: ", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	for _, subtest := range data.AppCompatPhysicalKeyboardTestCases[inputMethod] {
		validateAction := uiauto.Combine("validate pk typing in crosh",
			util.ClearTextFieldViaClickingBackspace(kb, data.LongestInputLength),
			kb.TypeSequenceAction(subtest.LocationKeySeq),
			ud.WaitUntilExists(uidetection.TextBlock(strings.Split(subtest.ExpectedText, " "))),
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
