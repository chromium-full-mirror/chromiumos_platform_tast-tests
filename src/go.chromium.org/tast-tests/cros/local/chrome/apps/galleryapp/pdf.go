// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package galleryapp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
)

var (
	pdfCanvas            = nodewith.Role(role.GraphicsDocument).Ancestor(RootFinder)
	drawSignatureCanvas  = nodewith.Name("Draw your signature in this area").Role(role.Canvas).Ancestor(RootFinder)
	placeSignatureButton = nodewith.Name("Place signature").Role(role.Button).Ancestor(RootFinder)
	pdfViewerWindow      = nodewith.NameStartingWith("Gallery - PDF Viewer").Role(role.Window)
	embeddedObject       = nodewith.Role("embeddedObject").Ancestor(pdfViewerWindow).First()
)

// DismissPDFDialog returns a function that dismisses PDF dialog.
func (g *Gallery) DismissPDFDialog() uiauto.Action {
	ui := g.ui
	dialog := nodewith.Name("Easily open and edit PDF files").Role(role.Dialog).Ancestor(RootFinder)
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(dialog)
	return uiauto.Retry(3, uiauto.Combine("dismiss PDF dialog",
		g.WaitUntilSpinnerGone(),
		ui.WaitUntilAnyExists(okButton, pdfCanvas),
		uiauto.IfSuccessThen(ui.Exists(okButton),
			ui.DoDefaultUntil(okButton,
				ui.WithTimeout(5*time.Second).WaitUntilGone(okButton)),
		)),
	)
}

// WaitPDFOpened returns a function that waits PDF file opened.
func (g *Gallery) WaitPDFOpened() uiauto.Action {
	return uiauto.Combine("wait PDF opened",
		g.WaitUntilSpinnerGone(),
		g.ui.WaitUntilExists(pdfCanvas),
	)
}

// WaitPDFViewerOpened returns a function that waits PDF viewer opened.
func (g *Gallery) WaitPDFViewerOpened() uiauto.Action {
	return uiauto.Combine("wait PDF viewer opened",
		g.ui.WithTimeout(longUITimeout).WaitUntilExists(embeddedObject),
	)
}

// AddTextToPDF returns a function that adds text to the PDF.
func (g *Gallery) AddTextToPDF(kb *input.KeyboardEventWriter, location coords.Point, text string) uiauto.Action {
	ui := g.ui
	addTextButton := nodewith.Name("Add text").Role(role.ToggleButton).Ancestor(RootFinder)
	fontHeading := nodewith.Name("Font").Role(role.Heading).Ancestor(RootFinder)
	textField := nodewith.Name("Annotation text field").Role(role.TextField).Ancestor(RootFinder)
	return uiauto.Retry(3,
		uiauto.NamedCombine(fmt.Sprintf("add text %q", text),
			uiauto.IfSuccessThen(ui.Gone(fontHeading), ui.LeftClick(addTextButton)),
			ui.WithTimeout(30*time.Second).WaitUntilExists(fontHeading),
			// Double click at location.
			ui.RetryUntil(ui.MouseClickAtLocation(2, location),
				ui.WithTimeout(5*time.Second).WaitUntilExists(textField)),
			kb.TypeAction(text),
		))
}

// Sign returns a function that clicks 'Sign' button.
func (g *Gallery) Sign() uiauto.Action {
	signButton := nodewith.Name("Sign").Role(role.ToggleButton).Ancestor(RootFinder)
	return uiauto.NamedCombine("create signature",
		g.ui.LeftClick(signButton),
		g.ui.WaitUntilAnyExists(drawSignatureCanvas, placeSignatureButton),
	)
}

// CreateSignature returns a function that creates signature for PDF.
func (g *Gallery) CreateSignature(signPoints []coords.Point) uiauto.Action {
	return uiauto.IfSuccessThen(g.ui.Exists(drawSignatureCanvas),
		uiauto.NamedCombine("create signature",
			g.draw(drawSignatureCanvas, signPoints),
			g.Done(),
		))
}

// PlaceSignature returns a function that places existing signature into
// the PDF.
func (g *Gallery) PlaceSignature() uiauto.Action {
	return uiauto.NamedCombine("place signature",
		g.ui.LeftClick(placeSignatureButton),
	)
}

// DrawOnPDF returns a function that draws pattern on the PDF through
// points.
func (g *Gallery) DrawOnPDF(points []coords.Point) uiauto.Action {
	ui := g.ui
	drawButton := nodewith.Name("Draw").Role(role.ToggleButton).Ancestor(RootFinder)
	penButton := nodewith.Name("Pen").Role(role.ToggleButton).Ancestor(RootFinder)
	return uiauto.NamedCombine("draw",
		uiauto.IfSuccessThen(ui.Gone(penButton), ui.LeftClick(drawButton)),
		ui.WaitUntilExists(penButton),
		g.draw(pdfCanvas, points),
	)
}

// OpenPDFViewer returns a function that opens PDF viewer from Gallery.
func (g *Gallery) OpenPDFViewer() uiauto.Action {
	ui := g.ui
	openMoreOptions := func(ctx context.Context) error {
		moreOptionsFinder := nodewith.Name("More options").Ancestor(RootFinder)
		moreOptionsButton, err := ui.FindAnyExists(ctx,
			moreOptionsFinder.Role(role.PopUpButton),
			moreOptionsFinder.Role(role.ToggleButton))
		if err != nil {
			return err
		}
		return ui.DoDefault(moreOptionsButton)(ctx)
	}
	openInPDFViewerItem := nodewith.Name("Open in PDF viewer").Role(role.MenuItem)
	return uiauto.NamedCombine("open in PDF viewer",
		openMoreOptions,
		ui.DoDefault(openInPDFViewerItem),
		g.WaitPDFViewerOpened(),
	)
}

// ClosePDFViewer returns a function that closes PDF viewer.
func (g *Gallery) ClosePDFViewer() uiauto.Action {
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(pdfViewerWindow)
	return uiauto.NamedCombine("close PDF viewer",
		g.ui.LeftClick(closeButton),
		g.ui.WaitUntilGone(closeButton),
	)
}

// ClickOnPDFCanvas returns a function that clicks on PDF canvas.
func (g *Gallery) ClickOnPDFCanvas() uiauto.Action {
	return uiauto.NamedAction("click on PDF canvas",
		g.ui.LeftClick(pdfCanvas),
	)
}

// ClickOnPDFPage returns a function that clicks on PDF page.
func (g *Gallery) ClickOnPDFPage() uiauto.Action {
	return uiauto.NamedAction("click on PDF page",
		g.ui.LeftClick(embeddedObject),
	)
}
