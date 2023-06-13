// Copyright 2021 The ChromiumOS Authors
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
		Func:         PhysicalKeyboardEnglishTyping,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that physical keyboard can perform basic typing",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:input-tools-upstream", "group:intel-gating", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		SearchFlags:  util.SearchFlagsWithIMEAndScreenPlay([]ime.InputMethod{ime.EnglishUS}, []string{"screenplay-2dc011b8-41c9-451f-a47b-5e5e6a3d55bb"}),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:   fixture.ClamshellNonVK,
				ExtraAttr: []string{"group:intel-nda"},
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func PhysicalKeyboardEnglishTyping(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Add IME for testing.
	im := ime.EnglishUS

	s.Log("Set current input method to: ", im)
	if err := im.InstallAndActivate(tconn)(ctx); err != nil {
		s.Fatalf("Failed to set input method to %v: %v: ", im, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, im.Name)

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

	var subtests = []util.InputEval{
		{
			TestName:     "Mixed (alphanumeric, symbols, enter) typing",
			InputFunc:    keyboard.TypeAction("Hello!\nTesting 123."),
			ExpectedText: "Hello!\nTesting 123.",
		}, {
			TestName: "Backspace to delete",
			InputFunc: uiauto.Combine("type a string and Backspace",
				keyboard.TypeAction("abc"),
				keyboard.AccelAction("Backspace"),
			),
			ExpectedText: "ab",
		}, {
			TestName: "Ctrl+Backspace",
			InputFunc: uiauto.Combine("type a string and Ctrl+Backspace",
				keyboard.TypeAction("hello world"),
				keyboard.AccelAction("Ctrl+Backspace"),
			),
			ExpectedText: "hello ",
		}, {
			TestName: "Editing middle of text",
			InputFunc: uiauto.Combine("type strings and edit in the middle of text",
				keyboard.TypeAction("abc"),
				keyboard.AccelAction("Left"),
				keyboard.AccelAction("Backspace"),
				keyboard.TypeAction("bc ab"),
			),
			ExpectedText: "abc abc",
		},
	}

	var inputField = testserver.TextAreaInputField

	for _, subtest := range subtests {
		s.Run(ctx, subtest.TestName, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+subtest.TestName)

			if err := uiauto.UserAction(
				"Engish PK input",
				its.ValidateInputOnField(inputField, subtest.InputFunc, subtest.ExpectedText),
				uc, &useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: subtest.TestName,
						useractions.AttributeInputField:   string(inputField),
						useractions.AttributeFeature:      useractions.FeaturePKTyping,
					},
				},
			)(ctx); err != nil {
				s.Fatalf("Failed to validate keys input in %s: %v", inputField, err)
			}

			if err := its.ValidateInputOnField(inputField, subtest.InputFunc, subtest.ExpectedText)(ctx); err != nil {
				s.Fatalf("Failed to validate %s: %v", subtest.TestName, err)
			}
		})
	}
}
