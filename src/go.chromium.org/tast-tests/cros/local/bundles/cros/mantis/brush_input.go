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

type differenceRange struct {
	low, high float64
}

type brushInputTestParameters struct {
	brushName string
	diffRange differenceRange
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
		Data:         []string{constant.ImageTestFileName},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "extra_thin",
				Val: brushInputTestParameters{
					brushName: "Extra thin",
					diffRange: differenceRange{
						low:  0,
						high: 0.1,
					},
				},
			},
			{
				Name: "thin",
				Val: brushInputTestParameters{
					brushName: "Thin",
					diffRange: differenceRange{
						low:  0.1,
						high: 0.2,
					},
				},
			},
			{
				Name: "medium",
				Val: brushInputTestParameters{
					brushName: "Medium",
					diffRange: differenceRange{
						low:  0.2,
						high: 0.3,
					},
				},
			},
			{
				Name: "thick",
				Val: brushInputTestParameters{
					brushName: "Thick",
					diffRange: differenceRange{
						low:  0.45,
						high: 0.6,
					},
				},
			},
			{
				Name: "extra_thick",
				Val: brushInputTestParameters{
					brushName: "Extra thick",
					diffRange: differenceRange{
						low:  0.7,
						high: 0.85,
					},
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

	before, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
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

	after, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	diff := util.ImageDiffPercentage(before, after)
	if diff < params.diffRange.low || diff > params.diffRange.high {
		s.Fatal("The before and after difference does not fall within acceptable range: ", diff)
	}
}
