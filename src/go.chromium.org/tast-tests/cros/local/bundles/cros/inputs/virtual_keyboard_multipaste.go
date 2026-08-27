// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
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
		Func:         VirtualKeyboardMultipaste,
		Desc:         "Test multipaste virtual keyboard functionality",
		Contacts:     []string{"essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUS},
			[]string{
				"screenplay-6abbc898-0eb4-4a74-8ce8-c256f9af6773",
				"screenplay-01e80f4f-423b-43d5-8392-a71a82a3f7ef",
				"screenplay-1e8cfd3e-22b6-4bce-be2e-9e470d3ac969",
				"screenplay-9158e01d-34aa-48ed-ab88-42034d7f40a7",
				"screenplay-831e152b-12af-488c-94cc-ce4d61f8f479",
				"screenplay-162fdfb4-f54e-4fb1-bdbd-7a69133d5e5b",
			}),
		Params: []testing.Param{
			{
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "informational",
				Fixture:           fixture.TabletVK,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels, hwdep.SkipOnPlatform("puff", "fizz")),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func VirtualKeyboardMultipaste(ctx context.Context, s *testing.State) {
	const (
		text1        = "Hello world"
		text2        = "12345"
		expectedText = "Hello world12345"
	)

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Launch inputs test web server.
	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// Select the input field being tested.
	inputField := testserver.TextAreaInputField
	vkbCtx := vkb.NewContext(cr, tconn)
	touchCtx, err := touch.New(ctx, tconn)
	if err != nil {
		s.Fatal("Fail to get touch screen: ", err)
	}
	defer touchCtx.Close(ctx)

	if err := ash.SetClipboard(ctx, tconn, text1); err != nil {
		s.Fatal("Failed to set text1 to clipboard: ", err)
	}
	if err := ash.SetClipboard(ctx, tconn, text2); err != nil {
		s.Fatal("Failed to set text2 to clipboard: ", err)
	}

	actionName := "Input from VK multipaste clipboard"
	if err := uiauto.UserAction(
		actionName,
		uiauto.Combine("navigate to multipaste virtual keyboard and paste text",
			its.ClickFieldUntilVKShown(inputField),
			vkbCtx.SwitchToMultipaste(),
			uiauto.RetrySilently(3, uiauto.Combine("click on multipaste items",
				its.ClickFieldByDoDefault(inputField),
				its.Clear(inputField),
				vkbCtx.TapMultipasteItem(text1),
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), text1),
			)),
			vkbCtx.TapMultipasteItem(text2),
			util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), expectedText),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField: string(inputField),
				useractions.AttributeFeature:    useractions.FeatureMultiPaste,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Fail to paste text through multipaste virtual keyboard: ", err)
	}

	actionName = "Select then de-select item in multipaste clipboard"
	ui := uiauto.New(tconn)

	if err := uiauto.UserAction(
		actionName,
		uiauto.Combine("Select then de-select item in multipaste virtual keyboard",
			touchCtx.LongPress(vkb.MultipasteItemFinder.Name(text1)),
			ui.WithTimeout(3*time.Second).WaitUntilExists(vkb.MultipasteTrashFinder),
			vkbCtx.TapMultipasteItem(text1),
			ui.WithTimeout(3*time.Second).WaitUntilGone(vkb.MultipasteTrashFinder),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeInputField: string(inputField),
				useractions.AttributeFeature:    useractions.FeatureMultiPaste,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Fail to select then de-select item: ", err)
	}

	actionName = "Remove item in VK multipaste clipboard"
	if err := uiauto.UserAction(
		actionName,
		vkbCtx.DeleteMultipasteItem(touchCtx, text1),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureMultiPaste,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Fail to long press to select and delete item: ", err)
	}
}
