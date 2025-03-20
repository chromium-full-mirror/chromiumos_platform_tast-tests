// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	extraThinFileName  = "brush/extra_thin_20250318.png"
	thinFileName       = "brush/thin_20250318.png"
	mediumFileName     = "brush/medium_20250318.png"
	thickFileName      = "brush/thick_20250318.png"
	extraThickFileName = "brush/extra_thick_20250318.png"
	pixelDiffThreshold = 100
)

type brushInputTestParameters struct {
	brushName              string
	expectedResultFileName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: BrushInput,
		Desc: "Verify brush functionality",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName, extraThinFileName, thinFileName, mediumFileName, thickFileName, extraThickFileName},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "extra_thin",
				Val: brushInputTestParameters{
					brushName:              "Extra thin",
					expectedResultFileName: extraThinFileName,
				},
			},
			{
				Name: "thin",
				Val: brushInputTestParameters{
					brushName:              "Thin",
					expectedResultFileName: thinFileName,
				},
			},
			{
				Name: "medium",
				Val: brushInputTestParameters{
					brushName:              "Medium",
					expectedResultFileName: mediumFileName,
				},
			},
			{
				Name: "thick",
				Val: brushInputTestParameters{
					brushName:              "Thick",
					expectedResultFileName: thickFileName,
				},
			},
			{
				Name: "extra_thick",
				Val: brushInputTestParameters{
					brushName:              "Extra thick",
					expectedResultFileName: extraThickFileName,
				},
			},
		},
	})
}

func BrushInput(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(constant.ImageTestFileName), constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	if err := util.OpenEditWithAIPanel(ctx, ui); err != nil {
		s.Fatal("Failed to open edit with AI panel: ", err)
	}

	if err := util.WaitForDLCPreparation(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for DLC preparation: ", err)
	}

	removeBackgroundButton := nodewith.Role(role.Button).Name("Remove Background").Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(removeBackgroundButton)(ctx); err != nil {
		s.Fatal("Failed to click the remove background button: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	params := s.Param().(brushInputTestParameters)
	brushButton := nodewith.Role(role.RadioButton).Name(params.brushName).Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(brushButton)(ctx); err != nil {
		s.Fatal("Failed to click the brush button: ", err)
	}

	// Draw a line on the image.
	if err := util.DrawOnImage(ctx, tconn, ui); err != nil {
		s.Fatal("Cannot draw on the image: ", err)
	}

	result, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	expectedResult, err := util.FetchImage(s.DataPath(params.expectedResultFileName))
	if err != nil {
		s.Fatal("Failed to get the expected image result file: ", err)
	}

	diff := util.ImageDiff(result, expectedResult)
	if diff > pixelDiffThreshold {
		s.Fatal("The result and the expected result are different. Diff: ", diff)
	}
}
