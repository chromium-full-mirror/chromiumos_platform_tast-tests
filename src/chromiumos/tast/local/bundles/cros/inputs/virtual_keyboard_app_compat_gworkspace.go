// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	fixture "chromiumos/tast/local/bundles/cros/inputs/fixture/appcompat"
	"chromiumos/tast/local/bundles/cros/inputs/util"
	"chromiumos/tast/local/chrome/googleapps"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/touch"
	"chromiumos/tast/local/chrome/uiauto/vkb"
	"chromiumos/tast/local/chrome/useractions"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualKeyboardAppCompatGworkspace,
		Desc:         "Test inputs feature on virtual keyboard for google workspace",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		Attr:         []string{"group:mainline", "group:input-tools", "informational"},
		LacrosStatus: testing.LacrosVariantNeeded, // b:260291042 implement lacros fixture for google workspace testing
		SoftwareDeps: []string{"chrome", "google_virtual_keyboard"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance, ime.EnglishUS}),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "docs",
				Fixture: fixture.GoogleDocs,
			},
			{
				Name:    "sheets",
				Fixture: fixture.GoogleSheets,
			},
			{
				Name:    "slides",
				Fixture: fixture.GoogleSlides,
			},
		},
	})
}

func VirtualKeyboardAppCompatGworkspace(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.WorkspaceFixtData).Chrome
	tconn := s.FixtValue().(fixture.WorkspaceFixtData).TestAPIConn
	uc := s.FixtValue().(fixture.WorkspaceFixtData).UserContext
	vkbCtx := vkb.NewContext(cr, tconn)
	appRootWebAreaName := s.FixtValue().(fixture.WorkspaceFixtData).AppRootWebAreaName

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	languageTests := map[ime.InputMethod][]util.AppCompatTestCase{
		ime.FrenchFrance: {
			{
				TestName:    "glide typing",
				Description: "user to glide typing to input word Bonjour",
				Steps: uiauto.Combine("test",
					vkbCtx.GlideTyping(strings.Split("bonjour", ""),
						util.VerifyTextToBe(tconn, nil, "Bonjour", util.VerifyInScreenshot)),
				),
			},
			{
				TestName:    "accent key",
				Description: "user type héllo",
				Steps: uiauto.Combine("user type text Héllo",
					vkbCtx.TapKeyIgnoringCase("h"),
					vkbCtx.TapAccentKey("e", "é"),
					vkbCtx.TapKeys(strings.Split("llo", "")),
					util.VerifyTextToBe(tconn, nil, "Héllo", util.VerifyInScreenshot),
				),
			},
		},
		ime.EnglishUS: {
			{
				TestName:    "normal typing",
				Description: "user typing word English",
				Steps: uiauto.Combine("testing typing, user type text English",
					vkbCtx.TapKeyIgnoringCase("e"),
					vkbCtx.TapKeys(strings.Split("nglish", "")),
					util.VerifyTextToBe(tconn, nil, "English", util.VerifyInScreenshot),
				),
			},
		},
	}

	touchCtx, err := touch.New(ctx, tconn)
	if err != nil {
		s.Fatal("Fail to get touch screen: ", err)
	}
	defer touchCtx.Close()

	for inputMethod, subtests := range languageTests {
		if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
			s.Fatal("Fail to set input method: ", err)
		}

		uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)
		for _, subtest := range subtests {
			// trigger vk for each application
			switch appRootWebAreaName {
			case "Google Docs":
				canvas := nodewith.Role(role.Canvas)

				if err := vkbCtx.TapScreenTriggerVK(touchCtx, tconn, canvas)(ctx); err != nil {
					s.Fatal("Failed to trigger vk in google docs: ", err)
				}

			case "Google Slides":
				cursorText := nodewith.ClassName("cursor-text").Role(role.GenericContainer)
				textContent := nodewith.Role(role.Group).ClassName("sketchy-text-content").Ancestor(cursorText).First()

				if err := vkbCtx.TapScreenTriggerVK(touchCtx, tconn, textContent)(ctx); err != nil {
					s.Fatal("Failed to trigger vk in google slides: ", err)
				}

			case "Google Sheets":
				baner := nodewith.Role(role.Banner)
				application := nodewith.ClassName("cell-input").Ancestor(baner)
				textBox := nodewith.Role(role.InlineTextBox).Ancestor(application).First()

				if err := vkbCtx.TapScreenTriggerVK(touchCtx, tconn, textBox)(ctx); err != nil {
					s.Fatal("Failed to trigger vk in google sheets: ", err)
				}
			}

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

			// Clean up the text field.
			var err error
			switch appRootWebAreaName {
			case "Google Docs", "Google Slides":
				err = googleapps.DeleteDocsOrSlidesContent(ctx, tconn, appRootWebAreaName)
			case "Google Sheets":
				err = googleapps.DeleteCellValue(ctx, tconn)
			}
			if err != nil {
				s.Logf("Failed to clean up for %s", appRootWebAreaName) // it won't affect test itself.
			}
		}
	}
}
