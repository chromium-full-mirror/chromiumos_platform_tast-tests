// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mantis

import (
	"context"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/constant"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mantis/util"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type reimagineFeatureTestParameters struct {
	prompts []string
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
		SoftwareDeps: []string{"chrome", "dlc"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Data:         []string{constant.ImageTestFileName},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.LoggedInWithUpdateEngine,
		Params: []testing.Param{
			{
				Name: "gen_fill",
				Val: reimagineFeatureTestParameters{
					prompts: []string{"a cute cat"},
				},
			},
			{
				Name: "gen_fill_multiple_context",
				Val: reimagineFeatureTestParameters{
					prompts: []string{"a cute cat", "a row of mountain"},
				},
			},
			{
				Name: "inpainting",
				Val: reimagineFeatureTestParameters{
					prompts: []string{""},
				},
			},
		},
	})
}

func getLineLocation(ctx context.Context, ui *uiauto.Context) (lineStart, lineEnd coords.Point, err error) {
	canvasBounds, err := ui.ImmediateLocation(ctx, util.ImageCanvas)
	if err != nil {
		return lineStart, lineEnd, errors.Wrap(err, "unable to get canvas location")
	}

	lineStart = coords.NewPoint(rand.Intn(canvasBounds.Width)+canvasBounds.Left, rand.Intn(canvasBounds.Height)+canvasBounds.Top)
	lineEnd = coords.NewPoint(rand.Intn(canvasBounds.Width)+canvasBounds.Left, rand.Intn(canvasBounds.Height)+canvasBounds.Top)

	return lineStart, lineEnd, nil
}

func runAndVerifyReimagine(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, cr *chrome.Chrome, kb *input.KeyboardEventWriter, prompt string) error {
	// Take a screenshot before reimagine.
	imageBefore, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		return errors.Wrap(err, "failed to grab screenshot before reimagine")
	}

	editWithAIButton := nodewith.Role(role.ToggleButton).Name("Edit with AI").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(editWithAIButton)(ctx); err != nil {
		return errors.Wrap(err, "unable to click 'Edit with AI' button")
	}

	if err := util.WaitForProgressBar(ctx, tconn, ui); err != nil {
		testing.ContextLog(ctx, "Error while waiting for progress bar: ", err)
	}

	reimagineButton := nodewith.Role(role.Button).Name("Reimagine").Ancestor(galleryapp.RootFinder).First()
	if err := ui.DoDefault(reimagineButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the reimagine button")
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		testing.ContextLog(ctx, "Error while waiting for spinner: ", err)
	}

	start, end, err := getLineLocation(ctx, ui)
	if err != nil {
		return errors.Wrap(err, "failed to get line location for scribble input")
	}

	// Draw a scribble on the image.
	if err := util.DrawOnImageWithLocation(ctx, tconn, ui, start, end); err != nil {
		return errors.Wrap(err, "failed draw on the image")
	}

	// Input the text prompt.
	reimagineTextArea := nodewith.Role(role.TextField).NameContaining("Write the word or phrase").Ancestor(galleryapp.RootFinder)
	if err := ui.LeftClick(reimagineTextArea)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the prompt text area")
	}

	if err := kb.Type(ctx, prompt); err != nil {
		return errors.Wrap(err, "failed to type the text prompt")
	}

	if err := ui.DoDefault(reimagineButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the reimagine button")
	}

	if err := util.WaitForSpinner(ctx, tconn, ui); err != nil {
		testing.ContextLog(ctx, "Error while waiting for spinner: ", err)
	}

	doneButton := nodewith.Role(role.Button).Name("Done").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(doneButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the done button")
	}

	// Take a screenshot after reimagine.
	imageAfter, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		return errors.Wrap(err, "failed to grab screenshot after reimagine")
	}

	if util.ImageDiff(imageBefore, imageAfter) == 0 {
		return errors.Wrap(err, "the image before and after reimagine should not be identical")
	}

	return nil
}

func ReimagineFeature(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(fixture.Data).Chrome
	tconn := s.FixtValue().(fixture.Data).TestAPIConn
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	if err := util.DownloadAndOpenFileInGallery(ctx, cr, s.DataPath(constant.ImageTestFileName), constant.ImageTestFileName); err != nil {
		s.Fatal("Failed to download and open file in Gallery: ", err)
	}

	ui := uiauto.New(tconn)
	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	params := s.Param().(reimagineFeatureTestParameters)
	for _, prompt := range params.prompts {
		if err := runAndVerifyReimagine(ctx, ui, tconn, cr, kb, prompt); err != nil {
			s.Fatalf("Reimagine failed for prompt %q: %v", prompt, err)
		}
	}

	imageAfter, err := util.GrabCanvasArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Failed to grab screenshot after reimagine: ", err)
	}

	// Save the result image and close Gallery app.
	saveButton := nodewith.Role(role.Button).Name("Save").Ancestor(galleryapp.RootFinder)
	if err := ui.DoDefault(saveButton)(ctx); err != nil {
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
