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
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type reimagineFeatureTestParameters struct {
	withPrompt bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ReimagineFeature,
		Desc: "Verify reimagine functionality",
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
				Name: "gen_fill",
				Val: reimagineFeatureTestParameters{
					withPrompt: true,
				},
			},
			{
				Name: "inpainting",
				Val: reimagineFeatureTestParameters{
					withPrompt: false,
				},
			},
		},
	})
}

func ReimagineFeature(ctx context.Context, s *testing.State) {
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

	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	if err := uiauto.Combine("Trigger Mantis initialization by clicking on 'Edit with AI' button",
		ui.WithTimeout(time.Minute).WaitUntilExists(editWithAIButton),
		ui.LeftClick(editWithAIButton))(ctx); err != nil {
		s.Fatal("Unable to click 'Edit with AI' button: ", err)
	}

	if err := util.WaitForProgressBar(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for progress bar: ", err)
	}

	params := s.Param().(reimagineFeatureTestParameters)
	reimagineButton := nodewith.Role(role.Button).Name("Reimagine").Ancestor(galleryapp.RootFinder).First()
	if err := util.LeftClickButton(ctx, ui, reimagineButton); err != nil {
		s.Fatal("Failed to click the reimagine button: ", err)
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	// Draw a scribble on the image.
	if err := util.DrawOnImage(ctx, tconn, ui); err != nil {
		s.Fatal("Cannot draw on the image")
	}

	if params.withPrompt {
		// Set up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to find keyboard: ", err)
		}
		defer kb.Close(ctx)

		// Input the text prompt.
		reimagineTextArea := nodewith.Role(role.TextField).Name("What do you want to generate in the area?").Ancestor(galleryapp.RootFinder)
		if err := ui.LeftClick(reimagineTextArea)(ctx); err != nil {
			s.Fatal("Failed to click the prompt text area: ", err)
		}

		if err := kb.Type(ctx, "a cute cat"); err != nil {
			s.Fatal("Failed to type the text prompt: ", err)
		}
	}

	if err := util.LeftClickButton(ctx, ui, reimagineButton); err != nil {
		s.Fatal("Failed to click the reimagine button: ", err)
	}

	s.Log("Reimagine on process")
	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		s.Log("Error while waiting for spinner: ", err)
	}

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := util.LeftClickButton(ctx, ui, doneButton); err != nil {
		s.Fatal("Failed to click the done button: ", err)
	}

	// Take a screenshot after reimagine.
	imageAfter, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot after reimagine: ", err)
	}

	if util.ImageDiff(imageBefore, imageAfter) == 0 {
		s.Fatal("The image before and after reimagine should not be identical")
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
		s.Fatal("The image after reimagine and the saved image should be identical")
	}
}
