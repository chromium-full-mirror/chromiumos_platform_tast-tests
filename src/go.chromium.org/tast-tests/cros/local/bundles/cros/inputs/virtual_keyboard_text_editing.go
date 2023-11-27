// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardTextEditing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the virtual keyboard can insert and delete text after clicking between different text fields",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUS}),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
			{
				Name:              "informational",
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosTabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
			},
		},
	})
}

func VirtualKeyboardTextEditing(ctx context.Context, s *testing.State) {
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

	ui := uiauto.New(tconn)
	vkbCtx := vkb.NewContext(cr, tconn)

	inputMethod := ime.EnglishUS
	if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
		s.Fatal("Failed to set input method: ", err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	inputField := testserver.TextAreaInputField
	noCorrectionInputField := testserver.TextAreaNoCorrectionInputField
	inputTextFinder := nodewith.Role(role.InlineTextBox).Ancestor(inputField.Finder())
	noCorrectionInputFieldFinder := nodewith.Role(role.InlineTextBox).Ancestor(noCorrectionInputField.Finder())

	clickTextRightBound := func(textFinder *nodewith.Finder, endIndex int) uiauto.Action {
		return func(ctx context.Context) error {
			textBounds, err := ui.BoundsForRange(ctx, textFinder, 0, endIndex)
			if err != nil {
				return errors.Wrap(err, "failed to get text location")
			}
			return ui.MouseClickAtLocation(0, textBounds.RightCenter())(ctx)
		}
	}

	validateAction := uiauto.Combine("edit text using virtual keyboard",
		// Insert text into an input field.
		its.ClickFieldUntilVKShown(inputField),
		vkbCtx.TapKeys(strings.Split("Abcdfg", "")),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abcdfg"),

		// Insert text into a different input field.
		its.ClickFieldUntilVKShown(noCorrectionInputField),
		vkbCtx.TapKeys(strings.Split("abd", "")),
		util.WaitForFieldTextToBe(tconn, noCorrectionInputField.Finder(), "abd"),

		// Move cursor back to the middle of the original input field and type.
		clickTextRightBound(inputTextFinder, 4),
		vkbCtx.TapKeys(strings.Split("de", "")),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abcddefg"),

		// Move cursor to the middle of the other input field and type.
		clickTextRightBound(noCorrectionInputFieldFinder, 2),
		vkbCtx.TapKeys(strings.Split("c", "")),
		util.WaitForFieldTextToBe(tconn, noCorrectionInputField.Finder(), "abcd"),

		// Move cursor back to the middle of the original input field and delete with Backspace.
		clickTextRightBound(inputTextFinder, 5),
		vkbCtx.TapKey("backspace"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abcdefg"),

		// Move cursor to the middle other input field and insert Space.
		clickTextRightBound(noCorrectionInputFieldFinder, 2),
		vkbCtx.TapKey("space"),
		util.WaitForFieldTextToBe(tconn, noCorrectionInputField.Finder(), "ab cd"),

		// Move cursor to the end of the original input field and insert at the end.
		clickTextRightBound(inputTextFinder, 7),
		vkbCtx.TapKeys(strings.Split("hjij", "")),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abcdefghjij"),

		// Delete from the middle of the text with Backspace
		clickTextRightBound(inputTextFinder, 9),
		vkbCtx.TapKey("backspace"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abcdefghij"),

		// Insert in the middle with Space.
		clickTextRightBound(inputTextFinder, 5),
		vkbCtx.TapKey("space"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abcde fghij"),

		// Insert in the middle with Glide typing (inserts space automatically).
		clickTextRightBound(inputTextFinder, 3),
		vkbCtx.GlideTyping(strings.Split("hat", ""), util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abc hatde fghij")),

		// Replace selected text with Space.
		ui.SelectText(inputTextFinder, 1, 7),
		vkbCtx.TapKey("space"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "A de fghij"),

		// Delete selected text with Backspace.
		ui.SelectText(inputTextFinder, 5, 7),
		vkbCtx.TapKey("backspace"),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "A de hij"),

		// Replace selected text with typing.
		ui.SelectText(inputTextFinder, 1, 4),
		vkbCtx.TapKeys(strings.Split("bc", "")),
		util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abc hij"),

		// Replace selected text with Glide typing.
		ui.SelectText(inputTextFinder, 3, 4),
		vkbCtx.GlideTyping(strings.Split("hat", ""), util.WaitForFieldTextToBe(tconn, inputField.Finder(), "Abc hathij")),
	)

	if err := uiauto.UserAction("Edit text using virtual keyboard",
		validateAction,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:    useractions.FeatureVKTyping,
				useractions.AttributeInputField: string(inputField),
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to edit text using virtual keyboard: ", err)
	}
}
