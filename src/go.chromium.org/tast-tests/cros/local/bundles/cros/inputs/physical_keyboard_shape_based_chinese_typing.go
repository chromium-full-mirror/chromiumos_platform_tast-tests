// Copyright 2022 The ChromiumOS Authors
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

// pkShapeBasedChineseTestCase struct encapsulates parameters for each test.
type pkShapeBasedChineseTestCase struct {
	inputMethod       ime.InputMethod
	typingKeys        string
	expectedResult    string
	expectedCandidate string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardShapeBasedChineseTyping,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that shape-based Chinese physical keyboard works",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:input-tools-upstream", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-a079c2f5-d442-4731-ba7b-c058d6d07913",
			},
		},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "array",
				Fixture: fixture.ClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseArray,
					typingKeys:     "aaa lbj mc gds exxw pf alpe ajr .aad ame ",
					expectedResult: "三節外也關由面再行列",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.ChineseArray}),
			},
			{
				Name:    "cangjie",
				Fixture: fixture.ClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseCangjie,
					typingKeys:     "a jwj yrhhi hui hxyc oiar grmbc ",
					expectedResult: "日車謝鬼與倉頡",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.ChineseCangjie}),
			},
			{
				Name:    "dayi",
				Fixture: fixture.ClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseDayi,
					typingKeys:     "1 j 123 asox db/ ",
					expectedResult: "言月詐做易",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.ChineseDayi}),
			},
			{
				Name:    "quick",
				Fixture: fixture.ClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseQuick,
					typingKeys:     "a jw yr an is ",
					expectedResult: "日富這門成",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.ChineseQuick}),
			},
			{
				Name:    "wubi",
				Fixture: fixture.ClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseWubi,
					typingKeys:     "yge yygy ggll yygt gg tt ",
					expectedResult: "请文一方五笔",
				},
				ExtraSearchFlags: util.IMESearchFlags([]ime.InputMethod{ime.ChineseWubi}),
			},
			{
				Name:    "array_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseArray,
					typingKeys:     "aaa lbj mc gds exxw pf alpe ajr .aad ame ",
					expectedResult: "三節外也關由面再行列",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.ChineseArray}),
			},
			{
				Name:    "cangjie_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseCangjie,
					typingKeys:     "a jwj yrhhi hui hxyc oiar grmbc ",
					expectedResult: "日車謝鬼與倉頡",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.ChineseCangjie}),
			},
			{
				Name:    "dayi_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseDayi,
					typingKeys:     "1 j 123 asox db/ ",
					expectedResult: "言月詐做易",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.ChineseDayi}),
			},
			{
				Name:    "quick_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseQuick,
					typingKeys:     "a jw yr an is ",
					expectedResult: "日富這門成",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.ChineseQuick}),
			},
			{
				Name:    "wubi_lacros",
				Fixture: fixture.LacrosClamshellNonVK,
				Val: pkShapeBasedChineseTestCase{
					inputMethod:    ime.ChineseWubi,
					typingKeys:     "yge yygy ggll yygt gg tt ",
					expectedResult: "请文一方五笔",
				},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraSearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.ChineseWubi}),
			},
		},
	})
}

func PhysicalKeyboardShapeBasedChineseTyping(ctx context.Context, s *testing.State) {
	testCase := s.Param().(pkShapeBasedChineseTestCase)

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	im := testCase.inputMethod

	s.Log("Set current input method to: ", im)
	if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %v: %v: ", im, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, im.Name)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputField := testserver.TextAreaInputField

	validateAction := its.ValidateInputOnField(inputField, kb.TypeAction(testCase.typingKeys), testCase.expectedResult)

	if err := uiauto.UserAction(
		"Shape-based Chinese PK input",
		validateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeTestScenario: fmt.Sprintf(`type %q to get %q`, testCase.typingKeys, testCase.expectedResult),
				useractions.AttributeFeature:      useractions.FeaturePKTyping,
				useractions.AttributeInputField:   string(inputField),
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to verify shape-based typing: ", err)
	}
}
