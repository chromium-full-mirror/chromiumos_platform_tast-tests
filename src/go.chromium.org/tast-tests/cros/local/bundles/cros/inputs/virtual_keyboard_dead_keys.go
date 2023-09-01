// Copyright 2020 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// deadKeysTestCase struct encapsulates parameters for each Dead Keys test.
type deadKeysTestCase struct {
	inputMethod          ime.InputMethod
	typingKeys           []string
	expectedTypingResult string
}

// Combining diacritic Unicode characters used as key caps of VK dead keys.
const (
	acuteAccent = "\u0301"
	circumflex  = "\u0302"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardDeadKeys,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that dead keys on the virtual keyboard work",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-1dff4cf7-e5df-4037-a698-82af3960a3d5",
			},
		},
		Timeout: 5 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "french",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
				// "French - French keyboard" input method uses a compact-layout VK for
				// non-a11y mode where there's no dead keys, and a full-layout VK for
				// a11y mode where there's dead keys. To test dead keys on the VK of
				// this input method, a11y mode must be enabled.
				Fixture: fixture.ClamshellVK,
				Val: deadKeysTestCase{
					// "French - French keyboard" input method is decoder-backed. Dead keys
					// are implemented differently from those of a no-frills input method.
					inputMethod: ime.FrenchFrance,
					// TODO(b/162292283): Make vkb.TapKeys() less flaky when the VK changes
					// based on Shift and Caps states, then add Shift and Caps related
					// typing sequences to the test case.
					typingKeys:           []string{circumflex, "a"},
					expectedTypingResult: "â",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			},
			{
				Name:              "french_informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
				Fixture:           fixture.ClamshellVK,
				Val: deadKeysTestCase{
					inputMethod:          ime.FrenchFrance,
					typingKeys:           []string{circumflex, "a"},
					expectedTypingResult: "â",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
			},
			{
				Name:              "catalan",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
				// "Catalan keyboard" input method uses the same full-layout VK (that
				// has dead keys) for both a11y & non-a11y. Just use non-a11y here.
				Fixture: fixture.TabletVK,
				Val: deadKeysTestCase{
					// "Catalan keyboard" input method is no-frills. Dead keys are
					// implemented differently from those of a decoder-backed input method.
					inputMethod: ime.Catalan,
					// TODO(b/162292283): Make vkb.TapKeys() less flaky when the VK changes
					// based on Shift and Caps states, then add Shift and Caps related
					// typing sequences to the test case.
					typingKeys:           []string{acuteAccent, "a"},
					expectedTypingResult: "á",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.Catalan}),
			},
			{
				Name:              "catalan_informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
				Fixture:           fixture.TabletVK,
				Val: deadKeysTestCase{
					inputMethod:          ime.Catalan,
					typingKeys:           []string{acuteAccent, "a"},
					expectedTypingResult: "á",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.Catalan}),
			},
			{
				Name:              "french_lacros",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				Fixture:           fixture.LacrosClamshellVK,
				Val: deadKeysTestCase{
					inputMethod:          ime.FrenchFrance,
					typingKeys:           []string{circumflex, "a"},
					expectedTypingResult: "â",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
				ExtraAttr:        []string{"group:hw_agnostic"},
			},
			{
				Name:              "catalan_lacros",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				Fixture:           fixture.LacrosTabletVK,
				Val: deadKeysTestCase{
					inputMethod:          ime.Catalan,
					typingKeys:           []string{acuteAccent, "a"},
					expectedTypingResult: "á",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.Catalan}),
				ExtraAttr:        []string{"group:hw_agnostic"},
			},
		},
	})
}

func VirtualKeyboardDeadKeys(ctx context.Context, s *testing.State) {
	testCase := s.Param().(deadKeysTestCase)

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
		s.Fatalf("Failed to set input method to %q: %v: ", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	vkbCtx := vkb.NewContext(cr, tconn)
	inputField := testserver.TextAreaNoCorrectionInputField

	validateAction := uiauto.Combine("validate dead keys typing",
		its.ClickFieldUntilVKShown(inputField),
		vkbCtx.TapKeys(testCase.typingKeys),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), testCase.expectedTypingResult),
	)

	if err := uiauto.UserAction(
		"VK dead keys input",
		validateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField: string(inputField),
				useractions.AttributeFeature:    useractions.FeatureDeadKeys,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to verify input: ", err)
	}
}
