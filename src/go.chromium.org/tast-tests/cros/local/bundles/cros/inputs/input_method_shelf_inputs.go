// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input/voice"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var testMessages = []data.Message{
	data.VoiceMessageHello,
	data.HandwritingMessageHello,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputMethodShelfInputs,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test input functions triggered from IME tray",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
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
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVKStereoAloopLoaded,
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVKStereoAloopLoaded,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:input-tools-upstream"},
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

	// Setup CRAS Aloop for audio test.
	err := voice.ActivateAloopNodes(ctx, tconn, voice.LoopbackPlayBack, voice.LoopbackCapture)
	if err != nil {
		s.Fatal("Failed to load Aloop: ", err)
	}

	if err := imesettings.EnableInputOptionsInShelf(uc, true)(ctx); err != nil {
		s.Fatal("Failed to show input options in shelf: ", err)
	}

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)
	inputField := testserver.TextAreaInputField
	testIME := ime.DefaultInputMethod

	imeMenuTrayButtonFinder := nodewith.Name("IME menu button").Role(role.Button)
	voiceInputItem := nodewith.Name("Voice").HasClass("SystemMenuButton")
	voicePrivacyConfirmButton := nodewith.Name("Got it").HasClass("voice-got-it")
	handwritingInputItem := nodewith.Name("Handwriting").HasClass("SystemMenuButton")
	handwritingPrivacyConfirmButton := nodewith.Name("Got it").HasClass("button")
	emojiInputMenuItem := nodewith.Name("Emojis").HasClass("SystemMenuButton")

	voiceInputData, ok := data.VoiceMessageHello.GetInputData(testIME)
	if !ok {
		s.Fatal("Failed to get voice test data of input method: ", testIME)
	}
	hwInputData, ok := data.HandwritingMessageHello.GetInputData(testIME)
	if !ok {
		s.Fatal("Failed to get handwriting test data of input method: ", testIME)
	}

	voiceInputUserAction := func() uiauto.Action {
		scenario := "Voice input triggered from IME tray"

		verifyAudioInputAction := uiauto.Combine(scenario,
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			ui.LeftClick(imeMenuTrayButtonFinder),
			ui.LeftClick(voiceInputItem),
			ui.DoDefaultUntil(voicePrivacyConfirmButton, ui.WithTimeout(2*time.Second).WaitUntilGone(voicePrivacyConfirmButton)),
			uiauto.Sleep(time.Second),
			func(ctx context.Context) error {
				return audio.PlayWavToPCM(ctx, s.DataPath(voiceInputData.VoiceFile), "hw:Loopback,0")
			},
			util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), voiceInputData.ExpectedText),
		)

		return uiauto.UserAction("Voice input",
			verifyAudioInputAction,
			uc,
			&useractions.UserActionCfg{
				Callback: func(ctx context.Context, actionErr error) error {
					vkbCtx := vkb.NewContext(cr, tconn)
					return vkbCtx.HideVirtualKeyboard()(ctx)
				},
				Attributes: map[string]string{
					useractions.AttributeInputField:   string(inputField),
					useractions.AttributeTestScenario: scenario,
					useractions.AttributeFeature:      useractions.FeatureVoiceInput,
				},
				Tags: []useractions.ActionTag{useractions.ActionTagIMEShelf},
			},
		)
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
				ui.WithTimeout(5*time.Second).WaitUntilExists(handwritingPrivacyConfirmButton),
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
		{"voice", voiceInputUserAction()},
		{"handwriting", handwritingInputUserAction()},
		{"emoji", emojiInputUserAction()},
	}

	for _, subtest := range subTests {
		util.RunSubTest(ctx, s, cr, subtest.name, subtest.action)
	}
}
