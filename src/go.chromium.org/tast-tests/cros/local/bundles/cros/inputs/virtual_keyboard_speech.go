// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"path/filepath"
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input/voice"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var voiceTestMessages = []data.Message{data.VoiceMessageHello}
var voiceTestIMEs = []ime.InputMethod{
	ime.ChinesePinyin,
	ime.EnglishUS,
}
var voiceTestIMEsNewData = []ime.InputMethod{
	ime.AlphanumericWithJapaneseKeyboard,
	ime.Arabic,
	ime.EnglishUK,
	ime.EnglishUSWithInternationalKeyboard,
	ime.Japanese,
	ime.Korean,
	ime.EnglishSouthAfrica,
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardSpeech,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test voice input functionality on virtual keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		SoftwareDeps: []string{"inputs_deps", "chrome", "google_virtual_keyboard"},
		Attr:         []string{"group:mainline", "group:input-tools", "group:input-tools-upstream"},
		SearchFlags:  util.SearchFlagsWithIMEAndScreenPlay(voiceTestIMEs, []string{"screenplay-83e4e9bd-e3be-4c32-8f28-5ac8a64f1838"}),
		Data:         data.ExtractExternalFiles(voiceTestMessages, append(voiceTestIMEs, voiceTestIMEsNewData...)),
		Timeout:      time.Duration(len(voiceTestIMEs)+len(voiceTestIMEsNewData)) * time.Duration(len(voiceTestMessages)) * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.TabletVKStereoAloopLoaded,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				Val:               voiceTestIMEs,
				ExtraAttr:         []string{"group:input-tools-upstream"},
			},
			{
				Name:              "newdata", // This test will be merged into CQ once it is proved to be stable.
				Fixture:           fixture.TabletVKStereoAloopLoaded,
				Val:               voiceTestIMEsNewData,
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraAttr:         []string{"informational", "group:input-tools-upstream"},
				ExtraSearchFlags:  util.IMESearchFlags(voiceTestIMEsNewData),
			},
			{
				Name:              "informational",
				Fixture:           fixture.TabletVKStereoAloopLoaded,
				Val:               append(voiceTestIMEs, voiceTestIMEsNewData...),
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosTabletVKStereoAloopLoaded,
				Val:               append(voiceTestIMEs, voiceTestIMEsNewData...),
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				ExtraSoftwareDeps: []string{"lacros_stable", "lacros"},
				ExtraSearchFlags:  util.IMESearchFlags(voiceTestIMEsNewData),
				ExtraAttr:         []string{"informational"},
			},
		},
	})
}

func VirtualKeyboardSpeech(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	testIMEs := s.Param().([]ime.InputMethod)

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup CRAS Aloop for audio test.
	err := voice.ActivateAloopNodes(ctx, tconn, voice.LoopbackPlayBack, voice.LoopbackCapture)
	if err != nil {
		s.Fatal("Failed to load Aloop: ", err)
	}

	// Launch inputs test web server.
	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	// Select the input field being tested.
	inputField := testserver.TextAreaInputField
	vkbCtx := vkb.NewContext(cr, tconn)

	subtest := func(testName string, inputData data.InputData) func(ctx context.Context, s *testing.State) {
		return func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			// Use a shortened context for test operations to reserve time for cleanup.
			ctx, shortCancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer shortCancel()

			defer func(ctx context.Context) {
				outDir := filepath.Join(s.OutDir(), testName)
				faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, s.HasError, cr, "ui_tree_"+testName)

				vkbCtx.SwitchToKeyboard() // Return to text input after test.

				if err := vkbCtx.HideVirtualKeyboard()(ctx); err != nil {
					s.Log("Failed to hide virtual keyboard: ", err)
				}

			}(cleanupCtx)

			verifyAudioInputAction := uiauto.Combine("verify audio input",
				its.Clear(inputField),
				uiauto.Sleep(time.Second),
				its.ClickFieldUntilVKShown(inputField),
				vkbCtx.SwitchToVoiceInput(),
				func(ctx context.Context) error {
					return audio.PlayWavToPCM(ctx, s.DataPath(inputData.VoiceFile), "hw:Loopback,0")
				},
				util.WaitForFieldTextToBeIgnoringCase(tconn, inputField.Finder(), inputData.ExpectedText),
			)

			if err := uiauto.UserAction("Voice input",
				verifyAudioInputAction,
				uc,
				&useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeInputField: string(inputField),
						useractions.AttributeFeature:    useractions.FeatureVoiceInput,
					},
				},
			)(ctx); err != nil {
				s.Fatal("Failed to validate voice input: ", err)
			}
		}
	}
	// Run defined subtest per input method and message combination.
	util.RunSubtestsPerInputMethodAndMessage(ctx, uc, s, testIMEs, voiceTestMessages, subtest)
}
