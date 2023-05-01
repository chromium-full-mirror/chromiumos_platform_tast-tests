// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/inputs/fixture"
	"chromiumos/tast/local/bundles/cros/inputs/pre"
	"chromiumos/tast/local/bundles/cros/inputs/testserver"
	"chromiumos/tast/local/bundles/cros/inputs/util"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/useractions"
	"chromiumos/tast/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardKioskTyping,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that user can type on physical keyboard in kiosk mode",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr: []string{
			"group:input-tools",
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "inputs_deps", "chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.DefaultInputMethod}),
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.KioskNonVK,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosKioskNonVK,
			},
		},
	})
}

func PhysicalKeyboardKioskTyping(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(fixture.InputsKioskFixtData).TestAPIConn
	uc := s.FixtValue().(fixture.InputsKioskFixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	ui := uiauto.New(tconn)
	inputField := testserver.TextAreaInputField

	actionName := "PK find inputfield and type"
	if err := uiauto.UserAction(actionName,
		uiauto.Combine(actionName,
			ui.WaitUntilExists(inputField.Finder()),
			ui.MakeVisible(inputField.Finder()),
			ui.LeftClick(inputField.Finder()),
			keyboard.TypeAction("abcdefghijklmnopqrstuvwxyz01234! ABCDEFGHIJKLMNOPQRSTUVWXYZ01234!"),
			util.WaitForFieldTextToBe(tconn, inputField.Finder(), "abcdefghijklmnopqrstuvwxyz01234! ABCDEFGHIJKLMNOPQRSTUVWXYZ01234!"),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeaturePKTyping,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate typing output: ", err)
	}
}
