// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
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
		Func:         PhysicalKeyboardAltgr,
		Desc:         "Checks that user can lock altgr modifier key on physical keyboard",
		Contacts:     []string{"essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{
				ime.EnglishUSWithInternationalKeyboard,
				ime.Swedish,
			},
			[]string{
				"screenplay-b70c86ef-bb20-4db4-baea-c59fff90d7c3",
				"screenplay-c9ace9f5-1d18-4acb-b85e-4598722b8cbc",
			}),
		Timeout: 5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVK,
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(pre.InputsStableModels),
			},
			{
				Name:              "informational",
				Fixture:           fixture.ClamshellNonVK,
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels),
			},
		},
	})
}

func PhysicalKeyboardAltgr(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	testCases := []struct {
		inputMethod         ime.InputMethod
		typeAction          string
		expectedText        string
		expectedShiftedText string
	}{
		{
			inputMethod:         ime.EnglishUSWithInternationalKeyboard,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789",
			expectedText:        "áb©ðéfghíjœøµñóöä®ßþúvåxüæ’¡²³¤€¼½¾‘",
			expectedShiftedText: "ÁB¢ÐÉFGHÍJŒØµÑÓÖÄ®§ÞÚVÅXÜÆ£",
		},
		{
			inputMethod:         ime.Swedish,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789",
			expectedText:        "ª”©ð€đŋħ→łµnœþ@®ßþ↓“ł»←«}¡@£$€¥{[]",
			expectedShiftedText: "º’©Ð¢ªŊĦıŁºNŒÞΩ®§Þ↑‘Ł>¥<°¹²³¼¢⅝÷«»",
		},
		{
			inputMethod:         ime.Norwegian,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789",
			expectedText:        "ª”©ð€đŋħ→łµnœπ@®ßþ↓“ł»←«}¡@£$½¥{[]",
			expectedShiftedText: "º’©Ð¢ªŊĦıŁºNŒΠΩ™§Þ↑‘Ł>¥<°¹²³¼‰⅝÷«»",
		},
		{
			inputMethod:         ime.EnglishUK,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789",
			expectedText:        "á”çðéđŋħíłµnóþ@¶ßŧú“ẃ»ý«}¹€½[]",
			expectedShiftedText: "Á’ÇÐÉªŊĦÍŁºNÓÞΩ®§ŦÚ‘Ẃ>Ý<°¡½⅓¼⅜⅝⅞™±",
		},
		{
			inputMethod:         ime.Polish,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789",
			expectedText:        "ą”ćðęæŋ’→ə…łµńóþπ©śß↓„œź←ż»≠²³¢€½§·«",
			expectedShiftedText: "Ą“ĆÐĘÆŊ•↔Ə∞ŃÓÞΩ®Ś™↑‘ŒŹ¥Ż°¡¿£¼‰∧≈¾±",
		},
		{
			inputMethod:         ime.DutchNetherlands,
			typeAction:          "abcdefghijklmnopqrstuvwxyz0123456789",
			expectedText:        "áb©ðéfghíjœøµñóöä®ßþúvåxüæ’¡²³¤€¼½¾‘",
			expectedShiftedText: "ÁB¢ÐÉFGHÍJŒØµÑÓÖÄ®§ÞÚVÅXÜÆ£",
		},
		{
			inputMethod:         ime.EnglishIndia,
			typeAction:          "4",
			expectedText:        "₹",
			expectedShiftedText: "",
		},
		{
			// CrOS 1P rule-based input method with AltGr character assignments by
			// layout remaps at IME layer (see https://crbug.com/243243592).
			inputMethod:         ime.TamilWithInScriptKeyboard,
			typeAction:          "0123456789",
			expectedText:        "௦௧௨௩௪௫௬௭௮௯",
			expectedShiftedText: "௦௧௨௩௪௫௬௭௮௯",
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	for _, testcase := range testCases {
		name := "PKAltgrModifierWorksFor" + testcase.inputMethod.ShortLabel
		scenario := "Verify PK Altgr Modifier Works For " + testcase.inputMethod.Name

		s.Run(ctx, name, func(ctx context.Context, s *testing.State) {
			// Reset Altgr, in case Altgr is in a held-down state (if release action did not get run due to failures)
			defer keyboard.AccelAction("RightAlt")(cleanupCtx)
			defer keyboard.AccelAction("Shift")(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(name))
			im := testcase.inputMethod

			s.Log("Set current input method to: ", im)
			if err := im.InstallAndActivateUserAction(uc)(ctx); err != nil {
				s.Fatalf("Failed to set input method to %v: %v: ", im, err)
			}

			if err := uiauto.UserAction("Verify PK Altgr Modifer Output",
				uiauto.Combine("Verify PK Altgr Modifier Output",
					its.Clear(inputField),
					its.ClickFieldAndWaitForActive(inputField),
					keyboard.AccelPressAction("RightAlt"),
					keyboard.TypeAction(testcase.typeAction),
					util.WaitForFieldTextToBe(tconn, inputField.Finder(), testcase.expectedText),
					its.Clear(inputField),
					its.ClickFieldAndWaitForActive(inputField),
					keyboard.AccelPressAction("Shift"),
					keyboard.TypeAction(testcase.typeAction),
					keyboard.AccelReleaseAction("RightAlt"),
					keyboard.AccelReleaseAction("Shift"),
					util.WaitForFieldTextToBe(tconn, inputField.Finder(), testcase.expectedShiftedText),
				),
				uc,
				&useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: scenario,
						useractions.AttributeFeature:      useractions.FeaturePKTyping,
						useractions.AttributeInputMethod:  im.Name,
					},
				},
			)(ctx); err != nil {
				s.Fatal("Failed to validate altgr: ", err)
			}
		})
	}
}
