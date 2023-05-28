// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardBasicTyping,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that user can do basic typing physical keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:criticalstaging", "informational"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishIndia, ime.EnglishPakistan}),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVK,
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
		},
	})
}

func PhysicalKeyboardBasicTyping(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	testCases := []struct {
		inputMethod         ime.InputMethod
		typeAction          string
		expectedText        string
		expectedShiftedText string
	}{
		{
			inputMethod:         ime.EnglishIndia,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789-=[]\\;',./",
			expectedText:        "abcdefghijklmnopqrstuvwxyz0123456789-=[]\\;',./",
			expectedShiftedText: "ABCDEFGHIJKLMNOPQRSTUVWXYZ)!@#$%^&*(_+{}|:\"<>?",
		},
		{
			inputMethod:         ime.EnglishPakistan,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789-=[]\\;',./",
			expectedText:        "abcdefghijklmnopqrstuvwxyz0123456789-=[]\\;',./",
			expectedShiftedText: "ABCDEFGHIJKLMNOPQRSTUVWXYZ)!@#$%^&*(_+{}|:\"<>?",
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	for _, testcase := range testCases {
		name := "PKBasicTypingWorksFor" + testcase.inputMethod.ShortLabel
		scenario := "Verify PK Basic Typing Works For " + testcase.inputMethod.Name

		s.Run(ctx, name, func(ctx context.Context, s *testing.State) {
			// Reset Shift, in case Shift is in a held-down state (if release action did not get run due to failures)
			defer keyboard.AccelAction("Shift")(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(name))
			im := testcase.inputMethod

			s.Log("Set current input method to: ", im)
			if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
				s.Fatalf("Failed to set input method to %v: %v: ", im, err)
			}

			if err := uiauto.UserAction("Verify PK Basic Typing Output",
				uiauto.Combine("Verify PK Basic Typing Output",
					its.Clear(inputField),
					its.ClickFieldAndWaitForActive(inputField),
					keyboard.TypeAction(testcase.typeAction),
					util.WaitForFieldTextToBe(tconn, inputField.Finder(), testcase.expectedText),
					its.Clear(inputField),
					its.ClickFieldAndWaitForActive(inputField),
					keyboard.AccelPressAction("Shift"),
					keyboard.TypeAction(testcase.typeAction),
					keyboard.AccelReleaseAction("Shift"),
					util.WaitForFieldTextToBe(tconn, inputField.Finder(), testcase.expectedShiftedText),
				),
				uc,
				&useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: scenario,
						useractions.AttributeFeature:      useractions.FeaturePKTyping,
						useractions.AttributeInputMethod:  im.Name,
					},
				},
			)(ctx); err != nil {
				s.Fatal("Failed to validate basic typing: ", err)
			}
		})
	}
}
