// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testrunner"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var testMessages = []data.Message{
	data.HandwritingMessageHello,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputMethodShelfInputs,
		Desc:         "Test input functions triggered from IME tray",
		Contacts:     []string{"essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal", "google_virtual_keyboard"},
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.DefaultInputMethod},
			[]string{
				"screenplay-e5f1d945-6b71-4011-994f-9f7a2c75f81d",
				"screenplay-3d7cd04b-6f65-4667-ab65-5991602b7b8a",
				"screenplay-7eb022ee-5490-4196-a8b5-ae23c9673a1f",
			}),
		Data:    data.ExtractExternalFiles(testMessages, []ime.InputMethod{ime.DefaultInputMethod}),
		Timeout: 5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVKStereoAloopLoaded,
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModelsForHandwriting...),
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVKStereoAloopLoaded,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModelsForHandwriting),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func InputMethodShelfInputs(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := imesettings.EnableInputOptionsInShelf(uc, true)(ctx); err != nil {
		s.Fatal("Failed to show input options in shelf: ", err)
	}

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)
	inputField := testserver.TextAreaInputField
	testIME := ime.DefaultInputMethod

	imeMenuTrayButtonFinder := nodewith.Name("IME menu button").Role(role.Button)
	handwritingInputItem := nodewith.Name("Handwriting").HasClass("SystemMenuButton")
	handwritingPrivacyConfirmButton := nodewith.Name("Got it").HasClass("button")
	emojiInputMenuItem := nodewith.Name("Emojis").HasClass("SystemMenuButton")

	hwInputData, ok := data.HandwritingMessageHello.GetInputData(testIME)
	if !ok {
		s.Fatal("Failed to get handwriting test data of input method: ", testIME)
	}

	handwritingInputUserAction := func() uiauto.Action {
		scenario := "Verify handwriting input triggered from IME tray"

		hwFilePath := s.DataPath(hwInputData.HandwritingFile)
		verifyHandWritingInputAction := uiauto.Combine(scenario,
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			ui.LeftClick(imeMenuTrayButtonFinder),
			ui.LeftClick(handwritingInputItem),
			// The privacy dialog does not appear on all devices.
			uiauto.IfSuccessThen(
				ui.WithTimeout(20*time.Second).WaitUntilExists(handwritingPrivacyConfirmButton),
				ui.DoDefaultUntil(handwritingPrivacyConfirmButton, ui.WithTimeout(2*time.Second).WaitUntilGone(handwritingPrivacyConfirmButton)),
			),
			func(ctx context.Context) error {
				hwCtx, err := vkb.NewContext(cr, tconn).NewHandwritingContext(ctx)
				if err != nil {
					return errors.Wrap(err, "failed to initiate handwriting context")
				}
				return uiauto.Combine(scenario,
					its.WaitForHandwritingEngineReadyOnField(hwCtx, inputField, hwFilePath),
					hwCtx.DrawStrokesFromFile(hwFilePath),
					util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), hwInputData.ExpectedText),
				)(ctx)
			},
		)

		return uiauto.UserAction("Handwriting",
			verifyHandWritingInputAction,
			uc,
			&useractions.UserActionCfg{
				Callback: func(ctx context.Context, actionErr error) error {
					vkbCtx := vkb.NewContext(cr, tconn)
					return vkbCtx.HideVirtualKeyboard()(ctx)
				},
				Attributes: map[string]string{
					useractions.AttributeInputField:   string(inputField),
					useractions.AttributeTestScenario: scenario,
					useractions.AttributeFeature:      useractions.FeatureHandWriting,
				},
				Tags: []useractions.ActionTag{useractions.ActionTagIMEShelf},
			},
		)
	}

	emojiInputUserAction := func() uiauto.Action {
		scenario := "Verify emoji input triggered from IME tray"

		inputEmoji := "😄"
		emojiPickerFinder := nodewith.Name("Emoji Picker").Role(role.RootWebArea)
		emojiItem := nodewith.Name(inputEmoji).Ancestor(emojiPickerFinder).First()

		verifyEmojiInputAction := uiauto.Combine(scenario,
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			ui.LeftClick(imeMenuTrayButtonFinder),
			ui.LeftClick(emojiInputMenuItem),
			ui.WithTimeout(30*time.Second).WaitUntilExists(emojiPickerFinder),
			its.DismissGifNudgeOverlay(),
			ui.WithTimeout(30*time.Second).LeftClick(emojiItem),
			util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), inputEmoji),
		)

		return uiauto.UserAction(
			"Input Emoji with Emoji Picker",
			verifyEmojiInputAction,
			uc,
			&useractions.UserActionCfg{
				Attributes: map[string]string{
					useractions.AttributeInputField:   string(inputField),
					useractions.AttributeTestScenario: scenario,
					useractions.AttributeFeature:      useractions.FeatureEmojiPicker,
				},
				Tags: []useractions.ActionTag{useractions.ActionTagIMEShelf},
			})
	}

	subTests := []struct {
		name   string
		action uiauto.Action
	}{
		{"handwriting", handwritingInputUserAction()},
		{"emoji", emojiInputUserAction()},
	}

	for _, subtest := range subTests {
		testrunner.RunSubTest(ctx, s, cr, subtest.name, subtest.action)
	}
}
