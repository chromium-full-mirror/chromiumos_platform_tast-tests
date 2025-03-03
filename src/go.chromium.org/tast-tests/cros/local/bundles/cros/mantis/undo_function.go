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

func init() {
	testing.AddTest(&testing.Test{
		Func: UndoFunction,
		Desc: "Verify undo button functionality",
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
	})
}

func UndoFunction(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(constant.ImageTestFileName), constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)

	// Take a screenshot before reimagine.
	imageBefore, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot before reimagine: ", err)
	}

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

	// Draw a scribble on the image.
	if err := util.DrawOnImage(ctx, tconn, ui); err != nil {
		s.Fatal("Cannot draw on the image")
	}

	if err := ui.DoDefault(reimagineButton)(ctx); err != nil {
		s.Fatal("Failed to click the reimagine button: ", err)
	}

	s.Log("Reimagine on process")
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(doneButton)(ctx); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	imageAfterReimagine, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	if util.ImageDiff(imageBefore, imageAfterReimagine) == 0 {
		s.Fatal("The image before and after reimagine should not be identical")
	}

	undoButton := nodewith.Role(role.Button).Name("Undo").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(undoButton)(ctx); err != nil {
		s.Fatal("Failed to click the undo button: ", err)
	}

	imageAfterUndo, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	if util.ImageDiff(imageBefore, imageAfterUndo) > 0 {
		s.Fatal("The image after clicking undo should be identical with the original image")
	}
}
