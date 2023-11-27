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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
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
		Func:         PhysicalKeyboardAutocorrectAccentKey,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that physical keyboard with accent keys can perform typing with autocorrects",
		Contacts: []string{
			"essential-inputs-gardener-oncall@google.com", // PoC
			"essential-inputs-team@google.com",
		},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func PhysicalKeyboardAutocorrectAccentKey(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputMethod := ime.FrenchFrance

	// Install IME and change auto-correct setting both need to wait for warm up.
	// Performing Install -> Setting -> Activate can save the wait time (15s) to speed up testing.
	if err := uiauto.NamedCombine("set current input method to: %q with PK autocorrect",
		inputMethod.Install(tconn),
		imesettings.SetPKAutoCorrection(uc, inputMethod, imesettings.AutoCorrectionOn),
		inputMethod.Activate(tconn),
	)(ctx); err != nil {
		s.Fatalf("Failed to set current input method to %q: %v", inputMethod, err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	defer func(ctx context.Context) {
		if err := inputMethod.ResetSettings(tconn)(ctx); err != nil {
			// Only log errors in cleanup.
			s.Log("Failed to reset IME settings: ", err)
		}
	}(cleanupCtx)

	inputField := testserver.TextAreaInputField

	const (
		typeKeys            = "d2jq"
		inputString         = "déja"
		autocorrectedString = "déjà"
	)

	action := uiauto.Combine("validate PK autocorrect with accent keys",
		its.ClearThenClickFieldAndWaitForActive(inputField),
		kb.TypeAction(typeKeys),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), inputString),
		kb.AccelAction("space"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), autocorrectedString+" "),
	)

	if err := uiauto.UserAction("PK input accent keys with auto-correct on",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeFeature:      useractions.FeatureAutoCorrection,
				useractions.AttributeTestScenario: fmt.Sprintf(`Input %q get %q`, inputString, autocorrectedString),
			}},
	)(ctx); err != nil {
		s.Fatal("Failed to validate PK autocorrect with accent keys: ", err)
	}
}
