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
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: EraseFeature,
		Desc: "Verify erase functionality",
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

func EraseFeature(ctx context.Context, s *testing.State) {
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

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	if err := util.WaitForDLCPreparation(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for DLC preparation: ", err)
	}

	eraseButton := nodewith.Role(role.Button).Name("Erase").Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(eraseButton)(ctx); err != nil {
		s.Fatal("Unable to open 'Erase' panel: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	// Draw a scribble on the image
	if err := util.DrawOnImage(ctx, tconn, ui); err != nil {
		s.Fatal("Cannot draw on the image: ", err)
	}

	// Click on the remove background button
	if err := ui.LeftClick(eraseButton)(ctx); err != nil {
		s.Fatal("Unable to click 'Erase' button: ", err)
	}

	s.Log("Erase on process")
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Fatal("Error while waiting for spinner: ", err)
	}

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	imageAfter, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot after reimagine: ", err)
	}

	// Save the result image and close Gallery app.
	saveButton := nodewith.Role(role.Button).Name("Save").Ancestor(galleryapp.RootFinder)
	if err := ui.LeftClick(saveButton)(ctx); err != nil {
		s.Fatal("Failed to click the save button: ", err)
	}
	savedText := nodewith.Role(role.StaticText).Name("Saved").Ancestor(galleryapp.RootFinder)
	if err := ui.WithTimeout(time.Second * 5).WaitUntilExists(savedText)(ctx); err != nil {
		s.Fatal("Failed to save image: ", err)
	}

	if err := util.CloseGallery(ctx, tconn); err != nil {
		s.Fatal("Failed to close Gallery: ", err)
	}

	// Reopen the result image in Gallery app.
	if err := util.OpenGalleryFromDownload(ctx, ui, tconn, constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to reopen Gallery: ", err)
	}

	// Take a screenshot of the saved.
	savedImage, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot of saved image: ", err)
	}

	if util.ImageDiff(imageAfter, savedImage) > 0 {
		s.Fatal("The result image and the saved image should be identical")
	}
}
