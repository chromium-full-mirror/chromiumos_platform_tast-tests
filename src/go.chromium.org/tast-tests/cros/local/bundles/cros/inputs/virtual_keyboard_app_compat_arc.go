// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vkb"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	fixture "go.chromium.org/tast-tests/cros/local/inputs/fixture/appcompat"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardAppCompatArc,
		Desc:         "Checks that virtual keyboard can perform typing in playstore search field",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:inputs_appcompat_arc_perbuild"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.PlayStoreWithVK,
	})
}

func VirtualKeyboardAppCompatArc(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.ArcFixtData).Chrome
	uc := s.FixtValue().(fixture.ArcFixtData).UserContext
	tconn := s.FixtValue().(fixture.ArcFixtData).TestAPIConn
	ud := s.FixtValue().(fixture.ArcFixtData).UIDetector
	kb := s.FixtValue().(fixture.ArcFixtData).Keyboard

	vkbCtx := vkb.NewContext(cr, tconn)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Adding more tests here.
	languageTests := map[ime.InputMethod][]util.AppCompatTestCase{
		ime.FrenchFrance: {
			{
				TestName:    "glide typing",
				Description: "glide typing word bonjour",
				Steps: uiauto.Combine("user glide typing bonjour",
					ud.Tap(uidetection.Word("Search")),
					vkbCtx.GlideTyping(strings.Split("bonjour", ""),
						ud.WaitUntilExists(uidetection.Word("bonjour").First())),
					util.ClearTextFieldViaClickingBackspace(kb, len("bonjour")),
				),
			},
			{
				TestName:    "accent key",
				Description: "user type héllo",
				Steps: uiauto.Combine("user type text Héllo",
					ud.Tap(uidetection.Word("Search")),
					vkbCtx.TapKeyIgnoringCase("h"),
					vkbCtx.TapAccentKey("e", "é"),
					vkbCtx.TapKeys(strings.Split("llo", "")),
					util.VerifyTextWithUIDetection(tconn, nil, "héllo"),
					vkbCtx.TapHideVitrualKeyboardButton(),
					util.ClearTextFieldViaClickingBackspace(kb, len("héllo")),
				),
			},
		},
		ime.EnglishUS: {
			{
				TestName:    "normal typing",
				Description: "user typing word English",
				Steps: uiauto.Combine("user type text English",
					ud.Tap(uidetection.Word("Search")),
					vkbCtx.TapKeyIgnoringCase("e"),
					vkbCtx.TapKeys(strings.Split("nglish", "")),
					vkbCtx.TapHideVitrualKeyboardButton(),
					util.VerifyTextWithUIDetection(tconn, nil, "english"),
					util.ClearTextFieldViaClickingBackspace(kb, len("english")),
				),
			},
		},
	}

	if err := ud.Tap(uidetection.Word("Search"))(ctx); err != nil {
		s.Fatal("Failed to trigger vk in playstore: ", err)
	}

	for inputMethod, subtests := range languageTests {
		if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
			s.Fatal("Fail to set input method: ", err)
		}

		uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)
		for _, subtest := range subtests {
			s.Run(ctx, subtest.TestName, func(ctx context.Context, s *testing.State) {
				if err := uiauto.UserAction(
					subtest.TestName,
					subtest.Steps,
					uc, &useractions.UserActionCfg{
						Attributes: map[string]string{
							useractions.AttributeTestScenario: subtest.TestName,
							useractions.AttributeFeature:      useractions.FeatureVKTyping,
						},
					},
				)(ctx); err != nil {
					s.Fatalf("Failed to validate %s typing in test %s: %v", inputMethod, subtest.TestName, err)
				}
			})
		}
	}
}
