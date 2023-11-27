// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	fixture "go.chromium.org/tast-tests/cros/local/inputs/fixture/appcompat"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/util"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardAppCompatGworkspace,
		Desc:         "Test inputs feature on physical keyboard for google workspace",
		Contacts:     []string{"essential-inputs-team@google.com", "essential-inputs-gardener-oncall@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools", "informational", "group:external-dependency"},
		LacrosStatus: testing.LacrosVariantExists,
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.FrenchFrance}),
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(pre.InputsStableModels),
		Params: []testing.Param{
			{
				Name:    "docs",
				Fixture: fixture.GoogleDocsNonVK,
			},
			{
				Name:    "sheets",
				Fixture: fixture.GoogleSheetsNonVK,
			},
			{
				Name:    "slides",
				Fixture: fixture.GoogleSlidesNonVK,
			},
			{
				Name:              "docs_lacros",
				Fixture:           fixture.LacrosGoogleDocsNonVK,
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:              "sheets_lacros",
				Fixture:           fixture.LacrosGoogleSheetsNonVK,
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:              "slides_lacros",
				Fixture:           fixture.LacrosGoogleSlidesNonVK,
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func PhysicalKeyboardAppCompatGworkspace(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.WorkspaceFixtData).Chrome
	tconn := s.FixtValue().(fixture.WorkspaceFixtData).TestAPIConn
	uc := s.FixtValue().(fixture.WorkspaceFixtData).UserContext
	appRootWebAreaName := s.FixtValue().(fixture.WorkspaceFixtData).AppRootWebAreaName

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	inputsMethodList := []ime.InputMethod{
		ime.FrenchFrance,
	}

	for _, inputMethod := range inputsMethodList {
		if err := inputMethod.InstallAndActivateUserAction(uc)(ctx); err != nil {
			s.Fatal("Fail to set input method: ", err)
		}

		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get keyboard: ", err)
		}
		defer kb.Close(ctx)

		uc.SetAttribute(useractions.AttributeInputMethod, inputMethod.Name)

		for _, subtest := range data.AppCompatPhysicalKeyboardTestCases[inputMethod] {
			s.Run(ctx, subtest.Description, func(ctx context.Context, s *testing.State) {
				if err := uiauto.UserAction(
					subtest.Description,
					uiauto.Combine(subtest.Description,
						kb.TypeSequenceAction(subtest.LocationKeySeq),
						util.VerifyTextWithUIDetection(tconn, nil, subtest.ExpectedText),
					),
					uc, &useractions.UserActionCfg{
						Attributes: map[string]string{
							useractions.AttributeTestScenario: subtest.Description,
							useractions.AttributeFeature:      useractions.FeatureVKTyping,
						},
					},
				)(ctx); err != nil {
					s.Fatalf("Failed to validate %s typing in test %s: %v", inputMethod, subtest.Description, err)
				}
			})

			//clean up
			var err error
			switch appRootWebAreaName {
			case "Google Docs", "Google Slides":
				err = googledocs.DeleteDocsOrSlidesContent(ctx, tconn, appRootWebAreaName)
			case "Google Sheets":
				err = googledocs.DeleteCellValue(ctx, tconn)
			}
			if err != nil {
				s.Logf("Failed to clean up for %s with error %v", appRootWebAreaName, err)
			}
		}
	}
}
