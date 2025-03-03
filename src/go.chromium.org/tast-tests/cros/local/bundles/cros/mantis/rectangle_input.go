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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const expectedResultFile = "rectangle_input_20250207.png"

func init() {
	testing.AddTest(&testing.Test{
		Func: RectangleInput,
		Desc: "Verify rectangle input functionality",
		Contacts: []string{
			"cros-mantis@google.com",
			"nurlitadf@google.com",
		},
		BugComponent: "b:1445284",
		Timeout:      constant.DefaultTestTimeout,
		SoftwareDeps: []string{"chrome", "chrome_internal", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName, expectedResultFile},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
	})
}

func RectangleInput(ctx context.Context, s *testing.State) {
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

	if err := util.WaitForProgressBar(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for progress bar: ", err)
	}

	reimagineButton := nodewith.Role(role.Button).Name("Reimagine").Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(reimagineButton)(ctx); err != nil {
		s.Fatal("Failed to click the reimagine button: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	rectangleButton := nodewith.Role(role.ToggleButton).Name("Rectangle").Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(rectangleButton)(ctx); err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to click the rectangle button: ", err)
	}

	canvasBounds, err := ui.ImmediateLocation(ctx, util.ImageCanvas)
	if err != nil {
		s.Fatal("Failed to get the canvas location: ", err)
	}

	startLocation := coords.NewPoint(
		canvasBounds.Left+10,
		canvasBounds.Top+10,
	)

	endLocation := coords.NewPoint(
		canvasBounds.CenterX(),
		canvasBounds.CenterY(),
	)

	// draw a rectangle on the image
	if err := mouse.Drag(tconn, startLocation, endLocation, 200*time.Millisecond)(ctx); err != nil {
		s.Fatal("Failed to move the mouse: ", err)
	}

	result, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	expectedResult, err := util.FetchImage(s.DataPath(expectedResultFile))
	if err != nil {
		s.Fatal("Failed to get the expected image result file: ", err)
	}

	diff := util.ImageDiff(result, expectedResult)
	if diff > 0 {
		s.Fatal("The result and the expected result are different. Diff: ", diff)
	}
}
