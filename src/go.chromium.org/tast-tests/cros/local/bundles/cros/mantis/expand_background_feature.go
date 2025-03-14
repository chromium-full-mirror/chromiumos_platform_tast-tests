// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type expandBackgroundFeatureTestParameters struct {
	ratioName      string
	expectedWidth  int64
	expectedHeight int64
}

const imageFileName = "a_cake_non_square_20250114.png"

func init() {
	testing.AddTest(&testing.Test{
		Func: ExpandBackgroundFeature,
		Desc: "Verify expand background functionality",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{imageFileName},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "square",
				Val: expandBackgroundFeatureTestParameters{
					ratioName:      "Ratio Square",
					expectedWidth:  1819,
					expectedHeight: 1819,
				},
			},
			{
				Name: "16_9",
				Val: expandBackgroundFeatureTestParameters{
					ratioName:      "Ratio 16 by 9",
					expectedWidth:  1820,
					expectedHeight: 1023,
				},
			},
			{
				Name: "4_3",
				Val: expandBackgroundFeatureTestParameters{
					ratioName:      "Ratio 4 by 3",
					expectedWidth:  1819,
					expectedHeight: 1364,
				},
			},
			{
				Name: "3_2",
				Val: expandBackgroundFeatureTestParameters{
					ratioName:      "Ratio 3 by 2",
					expectedWidth:  1819,
					expectedHeight: 1213,
				},
			},
		},
	})
}

func ExpandBackgroundFeature(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(imageFileName), imageFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	if err := util.OpenEditWithAIPanel(ctx, ui); err != nil {
		s.Fatal("Failed to open edit with AI panel: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	if err := util.WaitForProgressBar(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for progress bar: ", err)
	}

	expandBackgroundButton := nodewith.Role(role.Button).Name("Expand Background").Ancestor(galleryapp.RootFinder).First()
	if err := util.LeftClickButton(ctx, ui, expandBackgroundButton); err != nil {
		s.Fatal("Unable to open 'Expand Background' panel: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	// Pick a ratio
	params := s.Param().(expandBackgroundFeatureTestParameters)
	ratioButton := nodewith.Role(role.RadioButton).Name(params.ratioName).Ancestor(galleryapp.RootFinder).First()
	if err := util.LeftClickButton(ctx, ui, ratioButton); err != nil {
		s.Fatal("Unable to click ratio option: ", err)
	}

	// Click on the expand background button
	if err := uiauto.Combine("Start expand background process",
		ui.WithTimeout(time.Minute).WaitUntilExists(expandBackgroundButton),
		ui.LeftClick(expandBackgroundButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Expand Background' button: ", err)
	}

	s.Log("Expand Background on process")
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Fatal("Error while waiting for spinner: ", err)
	}

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	// Save the result image and close Gallery app.
	saveButton := nodewith.Role(role.Button).Name("Save").Ancestor(galleryapp.RootFinder)
	if err := util.LeftClickButton(ctx, ui, saveButton); err != nil {
		s.Fatal("Failed to click the save button: ", err)
	}
	savedText := nodewith.Role(role.StaticText).Name("Saved").Ancestor(galleryapp.RootFinder)
	if err := ui.WithTimeout(time.Second * 5).WaitUntilExists(savedText)(ctx); err != nil {
		s.Fatal("Failed to save image: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	imageAfter, err := util.FetchImage(downloadsPath + "/" + imageFileName)
	if err != nil {
		s.Fatal("Failed to get image result: ", err)
	}

	gotWidth := imageAfter.Bounds().Dx()
	gotHeight := imageAfter.Bounds().Dy()

	if gotWidth != int(params.expectedWidth) || gotHeight != int(params.expectedHeight) {
		s.Fatalf("Got unexpected image size. Expected width: %v, height: %v. Got width: %v, height: %v", params.expectedWidth, params.expectedHeight, gotWidth, gotHeight)
	}
}
