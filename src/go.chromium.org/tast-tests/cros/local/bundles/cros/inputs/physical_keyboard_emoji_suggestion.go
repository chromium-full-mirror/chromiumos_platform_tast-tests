// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/imesettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mountns"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardEmojiSuggestion,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks emoji suggestions with physical keyboard typing",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		// SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.EnglishUS}),
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.EnglishUS},
			[]string{
				"screenplay-a5a2d90f-88b4-47d6-ace9-c76003cd65bb",
				"screenplay-bdff0b9e-bfff-4ea4-836b-406901e04463",
			}),
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
				Fixture:           fixture.ClamshellNonVK,
			},
			{
				Name:              "guest",
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
				Fixture:           fixture.ClamshellNonVKInGuest,
			},
			{
				Name:              "incognito",
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
				Fixture:           fixture.ClamshellNonVK,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
				Fixture:           fixture.LacrosClamshellNonVK,
			},
			{
				Name:              "guest_lacros",
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
				Fixture:           fixture.LacrosClamshellNonVKInGuest,
			},
			{
				Name:              "incognito_lacros",
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
				Fixture:           fixture.LacrosClamshellNonVK,
			},
			{
				// Only run informational tests in consumer mode.
				Name:              "informational",
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
				Fixture:           fixture.ClamshellNonVK,
			},
		},
	})
}

func PhysicalKeyboardEmojiSuggestion(ctx context.Context, s *testing.State) {
	// In order for the "guest_lacros" case to work correctly, we need to
	// run the test body in the user mount namespace. See b/244513681.
	if err := mountns.WithUserSessionMountNS(ctx, func(ctx context.Context) error {
		physicalKeyboardEmojiSuggestion(ctx, s)
		return nil
	}); err != nil {
		s.Fatal("Failed to run test in correct mount namespace: ", err)
	}
}

func physicalKeyboardEmojiSuggestion(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// PK emoji suggestion only works in English(US).
	inputMethod := ime.EnglishUS

	// Activate function checks the current IME. It does nothing if the given input method is already in-use.
	// It is called here just in case IME has been changed in last test.
	if err := inputMethod.Activate(tconn)(ctx); err != nil {
		s.Fatal("Failed to set IME: ", err)
	}
	uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

	its, err := testserver.LaunchBrowserInMode(ctx, cr, tconn, s.FixtValue().(fixture.FixtData).BrowserType, strings.Contains(s.TestName(), "incognito"))
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	if strings.Contains(s.TestName(), "incognito") {
		uc.SetAttribute(useractions.AttributeIncognitoMode, strconv.FormatBool(true))
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	const inputField = testserver.TextInputField

	const (
		word  = "yum"
		emoji = "🤤"
	)

	emojiCandidateWindowFinder := nodewith.HasClass("SuggestionWindowView").Role(role.Window)
	emojiCharFinder := nodewith.Name(emoji).Ancestor(emojiCandidateWindowFinder).First()
	learnMoreFinder := nodewith.Name("Learn more").Ancestor(emojiCandidateWindowFinder).HasClass("ImageButton")
	ui := uiauto.New(tconn)

	validateInputUserAction := func(testScenario string, isEmojiSuggestionEnabled bool) uiauto.Action {
		action := uiauto.Combine("validate emoji suggestion",
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			kb.TypeAction(word),
			kb.AccelAction("SPACE"),
			func(ctx context.Context) error {
				if isEmojiSuggestionEnabled {
					// Select emoji and wait for the candidate window disappear.
					return uiauto.Combine("select emoji suggestion",
						ui.LeftClick(emojiCharFinder),
						ui.WaitUntilGone(emojiCandidateWindowFinder),
						util.WaitForFieldTextToBe(tconn, inputField.Finder(), word+" "+emoji),
					)(ctx)
				}
				// Otherwise check emoji suggestion window does not appear in 1s.
				// GoBigSleepLint: Sleep is necessary here, otherwise it immediately returns success because of UI reflection delay.
				testing.Sleep(ctx, time.Second)
				return uiauto.Combine("continue to input without emoji suggestion",
					ui.WaitUntilGone(emojiCandidateWindowFinder),
					kb.TypeAction(word),
					util.WaitForFieldTextToBe(tconn, inputField.Finder(), word+" "+word),
				)(ctx)
			},
		)
		return uiauto.UserAction(
			"Input emoji from suggestion",
			action,
			uc,
			&useractions.UserActionCfg{
				Attributes: map[string]string{
					useractions.AttributeTestScenario: testScenario,
					useractions.AttributeInputField:   string(inputField),
					useractions.AttributeFeature:      useractions.FeatureEmojiSuggestion,
				},
			},
		)
	}

	validateLearnMoreUserAction := uiauto.UserAction(
		"Learn more of emoji suggestion",
		uiauto.Combine(`validate "learn more" in emoji suggestion`,
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			// Use the first data to test "Learn more".
			kb.TypeAction(word),
			kb.AccelAction("SPACE"),
			ui.LeftClick(learnMoreFinder),
			ui.WaitUntilGone(emojiCandidateWindowFinder),
			ossettings.New(tconn).WaitUntilToggleOption(cr, imesettings.EmojiSuggestionsOption, true),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeTestScenario: `click "learn more" in emoji suggestion window to launch setting`,
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeFeature:      useractions.FeatureEmojiSuggestion,
			},
		},
	)

	if err := uiauto.Combine("validate emoji suggestion",
		validateInputUserAction("Emoji suggestion is enabled by default", true),
		imesettings.SetEmojiSuggestions(uc, false),
		validateInputUserAction("Emoji suggestion disabled in OS setting", false),
		imesettings.SetEmojiSuggestions(uc, true),
		validateInputUserAction("Emoji suggestion re-enabled in OS setting", true),
		validateLearnMoreUserAction,
	)(ctx); err != nil {
		s.Fatal("Failed to validate emoji suggestion: ", err)
	}
}
