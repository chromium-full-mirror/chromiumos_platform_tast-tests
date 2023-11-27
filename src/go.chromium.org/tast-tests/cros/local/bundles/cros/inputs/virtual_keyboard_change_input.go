// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
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
		Func:         VirtualKeyboardChangeInput,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that changing input method in different ways",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUS, ime.ChinesePinyin}),
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "tablet",
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
			},
			{
				Name:              "a11y",
				Fixture:           fixture.ClamshellVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream", "group:hw_agnostic"},
			},
			{
				Name:              "informational",
				ExtraAttr:         []string{"informational"},
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
			{
				Name:              "tablet_lacros",
				Fixture:           fixture.LacrosTabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
			{
				Name:              "a11y_lacros",
				Fixture:           fixture.LacrosClamshellVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func VirtualKeyboardChangeInput(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	inputMethod := ime.ChinesePinyin
	typingTestData, ok := data.TypingMessageHello.GetInputData(inputMethod)
	if !ok {
		s.Fatalf("Test Data for input method %v does not exist", inputMethod)
	}

	if err := inputMethod.Install(tconn)(ctx); err != nil {
		s.Fatalf("Failed to install input method %q: %v", inputMethod, err)
	}

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)
	vkbctx := vkb.NewContext(cr, tconn)

	inputField := testserver.TextAreaInputField
	inputMethodOption := vkb.NodeFinder.Name(inputMethod.Name).Role(role.StaticText)
	// String is changing to uppercase, so allow either o.
	vkLanguageMenuFinder := vkb.NodeFinder.NameRegex(regexp.MustCompile("[oO]pen keyboard menu"))

	validateAction := uiauto.Combine("verify changing input method on virtual keyboard",
		// Switch IME using virtual keyboard language menu.
		its.ClickFieldUntilVKShown(inputField),
		ui.LeftClick(vkLanguageMenuFinder),
		ui.LeftClick(inputMethodOption),
		ui.WaitUntilExists(vkb.NodeFinder.NameRegex(regexp.MustCompile(fmt.Sprintf("%s|%s", inputMethod.ShortLabel, inputMethod.Name))).Role(role.StaticText)),

		// Validate current input method change.
		inputMethod.WaitUntilActivated(tconn),

		// Validate typing test.
		vkbctx.TapKeysIgnoringCase(typingTestData.CharacterKeySeq),
		vkbctx.SelectFromSuggestion(typingTestData.ExpectedText),
		util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), typingTestData.ExpectedText),
	)

	if err := uiauto.UserAction("Switch input method on VK",
		validateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeTestScenario: fmt.Sprintf("Change input method to %q", inputMethod.Name),
				useractions.AttributeFeature:      useractions.FeatureIMEManagement,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to verify changing input method: ", err)
	}
}
