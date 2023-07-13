// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
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
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that user can do basic typing physical keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags:  util.IMESearchFlags(pkTypingTestIMEs),
		Timeout:      time.Duration(len(pkTypingTestIMEs)*len(pkTypingTestMessages)) * time.Minute,
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				Fixture:           fixture.ClamshellNonVK,
				ExtraAttr:         []string{"group:input-tools-upstream", "group:criticalstaging"},
			},
			{
				Name:              "informational",
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				Fixture:           fixture.ClamshellNonVK,
				ExtraSearchFlags:  util.IMESearchFlags(pkTypingTestIMEs),
			},
			{
				Name:              "lacros",
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
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

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	subtest := func(testName string, inputData data.InputData) func(ctx context.Context, s *testing.State) {
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
	util.RunSubtestsPerInputMethodAndMessage(ctx, uc, s, pkTypingTestIMEs, pkTypingTestMessages, subtest)
}
