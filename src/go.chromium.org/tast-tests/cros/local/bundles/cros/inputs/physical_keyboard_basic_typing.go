// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testrunner"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var pkTypingTestIMEs = []ime.InputMethod{
	ime.Arabic,
	ime.AlphanumericWithJapaneseKeyboard,
	ime.Cantonese,
	ime.ChineseCangjie,
	ime.ChinesePinyin,
	ime.EnglishCanada,
	ime.EnglishIndia,
	ime.EnglishPakistan,
	ime.EnglishSouthAfrica,
	ime.EnglishUK,
	ime.EnglishUS,
	ime.EnglishUSWithInternationalKeyboard,
	ime.FrenchFrance,
	ime.Japanese,
	ime.JapaneseWithUSKeyboard,
	ime.Khmer,
	ime.Korean,
	ime.Myanmar,
	ime.Sinhala,
	ime.SpanishSpain,
	ime.Swedish,
	ime.ThaiTis,
}

var pkTypingTestMessages = []data.Message{data.TypingMessageHello}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardBasicTyping,
		Desc:         "Checks that user can do basic typing physical keyboard",
		Contacts:     []string{"essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			pkTypingTestIMEs,
			[]string{
				"screenplay-fc41aee0-2fbd-444a-b082-0f65c4174896",
				"screenplay-b5d8c1f6-d4c4-456b-84e1-235b8150e149",
				"screenplay-b66def52-3ab4-46b8-ab97-cf16ee9be142",
				"screenplay-139275d8-6c5b-4fab-91f8-adaf843eae48",
			}),
		Timeout: time.Duration(len(pkTypingTestIMEs)*len(pkTypingTestMessages)) * time.Minute,
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				Fixture:           fixture.ClamshellNonVK,
				Val:               pkTypingTestIMEs,
				ExtraAttr:         []string{"group:input-tools-upstream", "group:release-health", "release-health_essential_inputs"},
			},
			{
				Name:              "informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				Fixture:           fixture.ClamshellNonVK,
				Val:               pkTypingTestIMEs,
				ExtraSearchFlags:  util.IMESearchFlags(pkTypingTestIMEs),
			},
			{
				Name:              "first_party_vietnamese",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				Fixture:           fixture.ClamshellNonVKWithFirstPartyVietnamese,
				Val:               []ime.InputMethod{ime.VietnameseTelex, ime.VietnameseVNI},
				ExtraAttr:         []string{"group:release-health", "release-health_essential_inputs"},
			},
		},
	})
}

func PhysicalKeyboardBasicTyping(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	subtest := func(testName string, inputData data.InputData, _ ime.InputMethod) func(ctx context.Context, s *testing.State) {
		return func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			// Use a shortened context for test operations to reserve time for cleanup.
			ctx, shortCancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer shortCancel()

			defer func(ctx context.Context) {
				outDir := filepath.Join(s.OutDir(), testName)
				faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, s.HasError, cr, "ui_tree_"+testName)
			}(cleanupCtx)

			if err := its.ValidateInputFieldForMode(uc, inputField, util.InputWithPK, inputData, s.DataPath)(ctx); err != nil {
				s.Fatal("Failed to validate physical keyboard input: ", err)
			}
		}
	}
	// Run defined subtest per input method and message combination.
	testrunner.RunSubtestsPerInputMethodAndMessage(ctx, uc, s, s.Param().([]ime.InputMethod), pkTypingTestMessages, subtest)
}
