// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

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
		Func:         PhysicalKeyboardTextEditing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that user can edit text with the physical keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:input-tools-upstream"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUS}),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.ClamshellNonVK,
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func PhysicalKeyboardTextEditing(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

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
	inputTextFinder := nodewith.Role(role.InlineTextBox).Ancestor(inputField.Finder())

	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)

	actionName := "text selection in PK"
	if err := uiauto.UserAction(actionName,
		uiauto.Combine(actionName,
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			keyboard.TypeAction("abc def ghi"),
			// abc def |ghi
			keyboard.AccelAction("Left"),
			keyboard.AccelAction("Left"),
			keyboard.AccelAction("Left"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 8, 8, ""),
			// abc def g|hi
			keyboard.AccelAction("Right"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 9, 9, ""),
			// abc def |ghi
			keyboard.AccelAction("Ctrl+Left"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 8, 8, ""),
			// abc def ghi|
			keyboard.AccelAction("Ctrl+Right"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 11, 11, ""),
			// abc def |ghi|
			keyboard.AccelAction("Ctrl+Shift+Left"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 8, 11, "ghi"),
			// abc def ghi|
			keyboard.AccelAction("Ctrl+Shift+Right"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 11, 11, ""),
			// abc |def| ghi
			ui.SelectText(inputTextFinder, 4, 7),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 4, 7, "def"),
			// abc |def |ghi
			keyboard.AccelAction("Shift+Right"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 4, 8, "def "),
			// abc |def| ghi
			keyboard.AccelAction("Shift+Left"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 4, 7, "def"),
			// abc |def| ghi
			keyboard.AccelAction("Esc"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 4, 7, "def"),
			// |abc def ghi
			keyboard.AccelAction("Up"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 0, 0, ""),
			// abc def ghi|
			keyboard.AccelAction("Down"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 11, 11, ""),
			// abc| def |abc
			ui.SelectText(inputTextFinder, 3, 8),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 3, 8, " def "),
			// abcxyz|ghi
			keyboard.TypeAction("xyz"),
			util.WaitForFieldTextToBe(tconn, inputField.Finder(), "abcxyzghi"),
			util.WaitForFieldSelectionToBe(its.Conn(), tconn, inputField.Finder(), 6, 6, ""),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeaturePKTyping,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate text selection: ", err)
	}
}
