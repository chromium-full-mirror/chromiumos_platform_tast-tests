// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/autocorrect"
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
		Func:         PhysicalKeyboardAutocorrect,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that physical keyboard can perform typing with autocorrects",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUS},
			[]string{
				"screenplay-0c72a440-b472-4375-a30d-d9985b193e7c",
				"screenplay-104f9d3e-b4cd-4cdb-82e5-c177484961b3",
			}),
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
		Params: []testing.Param{
			{
				Name:    "en_us_1",
				Fixture: fixture.ClamshellNonVK,
				Val: autocorrect.TestCase{
					InputMethod:  ime.EnglishUS,
					MisspeltWord: "helol",
					CorrectWord:  "hello",
					UndoMethod:   autocorrect.ViaPopupUsingPK,
				},
				ExtraAttr: []string{"group:input-tools-upstream", "group:cq-medium", "group:hw_agnostic"},
			},
			{
				Name:    "en_us_2",
				Fixture: fixture.ClamshellNonVK,
				Val: autocorrect.TestCase{
					InputMethod:  ime.EnglishUS,
					MisspeltWord: "wrold",
					CorrectWord:  "world",
					UndoMethod:   autocorrect.ViaPopupUsingMouse,
				},
				ExtraAttr: []string{"group:input-tools-upstream", "group:hw_agnostic"},
			},
			{
				Name:              "en_us_1_lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				Val: autocorrect.TestCase{
					InputMethod:  ime.EnglishUS,
					MisspeltWord: "helol",
					CorrectWord:  "hello",
					UndoMethod:   autocorrect.ViaPopupUsingPK,
				},
			},
			{
				Name:              "en_us_2_lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				Val: autocorrect.TestCase{
					InputMethod:  ime.EnglishUS,
					MisspeltWord: "wrold",
					CorrectWord:  "world",
					UndoMethod:   autocorrect.ViaPopupUsingMouse,
				},
				ExtraAttr: []string{"group:input-tools-upstream"},
			},
			// Test cases for other input methods can be added once the framework
			// supports more than just US-Qwerty layout.
		},
	})
}

func PhysicalKeyboardAutocorrect(ctx context.Context, s *testing.State) {
	testCase := s.Param().(autocorrect.TestCase)

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	inputMethod := testCase.InputMethod
	s.Logf("Set current input method to: %q", inputMethod)

	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %q: %v: ", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	defer func(ctx context.Context) {
		if err := inputMethod.ResetSettings(tconn)(ctx); err != nil {
			// Only log errors in cleanup.
			s.Log("Failed to reset IME settings: ", err)
		}
	}(cleanupCtx)

	const inputField = testserver.TextAreaInputField

	validatePKAutocorrectAction := uiauto.Combine("validate PK autocorrect",
		inputMethod.SetPKAutoCorrection(tconn, ime.AutoCorrectionModest),
		its.Clear(inputField),
		its.ClickFieldAndWaitForActive(inputField),
		keyboard.TypeAction(testCase.MisspeltWord),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.MisspeltWord),
		keyboard.TypeAction(" "),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.CorrectWord+" "),
	)

	if err := uiauto.UserAction("PK autocorrect",
		validatePKAutocorrectAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeTestScenario: fmt.Sprintf(`correct %q to %q`, testCase.MisspeltWord, testCase.CorrectWord),
				useractions.AttributeFeature:      useractions.FeatureAutoCorrection,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate PK autocorrect: ", err)
	}

	ui := uiauto.New(tconn)
	undoWindowFinder := nodewith.ClassName("UndoWindow").Role(role.Window)
	undoButtonFinder := nodewith.Name("Undo").Role(role.Button).Ancestor(undoWindowFinder)

	triggerUndoAction := uiauto.Combine("press left button to trigger AC undo",
		uiauto.Repeat(len(testCase.CorrectWord)/2+1, keyboard.AccelAction("Left")),
		ui.WaitUntilExists(undoButtonFinder),
	)

	if err := uiauto.UserAction("press LEFT key to trigger AC undo",
		triggerUndoAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField: string(inputField),
				useractions.AttributeFeature:    useractions.FeatureAutoCorrection,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to trigger AC undo: ", err)
	}

	var testScenario string
	var undoAction uiauto.Action
	switch testCase.UndoMethod {
	case autocorrect.ViaBackspace:
		// Not applicable for PK. Expect no undo upon Backspace.
		testScenario = "PK autocorrect non-undo via Backspace"
		undoAction = uiauto.Combine(testScenario,
			keyboard.AccelAction("Backspace"),
			util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.CorrectWord),
		)
	case autocorrect.ViaPopupUsingPK:
		testScenario = "PK autocorrect undo via popup using PK"
		undoAction = uiauto.Combine(testScenario,
			keyboard.AccelAction("Up"),
			keyboard.AccelAction("Enter"),
			util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.MisspeltWord+" "),
		)
	case autocorrect.ViaPopupUsingMouse:
		testScenario = "PK autocorrect undo via popup using mouse"
		undoAction = uiauto.Combine(testScenario,
			ui.LeftClick(undoButtonFinder),
			util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.MisspeltWord+" "),
		)
	}

	if err := uiauto.UserAction("PK autocorrect undo",
		undoAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeTestScenario: testScenario,
				useractions.AttributeFeature:      useractions.FeatureAutoCorrection,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate autocorrect undo: ", err)
	}
}
