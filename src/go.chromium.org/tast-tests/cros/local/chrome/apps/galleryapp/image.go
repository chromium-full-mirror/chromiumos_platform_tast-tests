// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package galleryapp

import (
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
)

// Rotate ​​is the option for image rotation.
type Rotate string

const (
	// RotateClockwise is an option to rotate the image clockwise.
	RotateClockwise Rotate = "Rotate clockwise"
	// RotateCounterClockwise is an option to rotate the image counterclockwise.
	RotateCounterClockwise Rotate = "Rotate counterclockwise"
)

// Ratio ​​is the option for image ratio.
type Ratio string

const (
	// RatioOriginal is an option to set image ratio to original.
	RatioOriginal Ratio = "Ratio Original"
	// RatioSquare is an option to set image ratio to square.
	RatioSquare Ratio = "Ratio Square"
	// Ratio16by9 is an option to set image ratio to 16 by 9.
	Ratio16by9 Ratio = "Ratio 16 by 9"
	// Ratio3by2 is an option to set image ratio to 3 by 2.
	Ratio3by2 Ratio = "Ratio 3 by 2"
	// Ratio4by3 is an option to set image ratio to 4 by 3.
	Ratio4by3 Ratio = "Ratio 4 by 3"
)

// Slider is the slider type of the lighting filters.
type Slider string

const (
	// SliderExposure is the exposure slider.
	SliderExposure Slider = "Exposure"
	// SliderContrast is the contrast slider.
	SliderContrast Slider = "Contrast"
	// SliderSaturation is the saturation slider.
	SliderSaturation Slider = "Saturation"
)

var (
	backlightDropTarget = nodewith.HasClass("backlight-drop-target").Ancestor(RootFinder)
	imageFinder         = nodewith.Role(role.Image).Ancestor(backlightDropTarget)
	imageCanvas         = imageFinder.First()
)

// WaitImageOpened returns a function that waits until image file is opened.
func (g *Gallery) WaitImageOpened() uiauto.Action {
	return uiauto.Combine("wait image opened",
		g.WaitUntilSpinnerGone(),
		g.ui.WaitUntilExists(imageCanvas),
	)
}

// SetCropAndRotate returns a function that opens 'Crop & rotate' and
// rotates the image clockwise or counterclockwise by a specific count.
func (g *Gallery) SetCropAndRotate(r Rotate, count int) uiauto.Action {
	cropRotateButton := nodewith.Name("Crop & rotate").Role(role.ToggleButton).Ancestor(RootFinder)
	rotateButton := nodewith.Name(string(r)).Role(role.Button).Ancestor(RootFinder)
	actionName := fmt.Sprintf("click %q for %v times", r, count)
	return uiauto.NamedCombine(actionName,
		g.ui.LeftClick(cropRotateButton),
		uiauto.Repeat(count, g.ui.WithInterval(time.Second).LeftClick(rotateButton)),
	)
}

// CloseCropAndRotate returns a function that closes 'Crop & rotate'.
func (g *Gallery) CloseCropAndRotate() uiauto.Action {
	main := nodewith.Role(role.Main).Ancestor(RootFinder)
	closeButton := nodewith.Name("Close").Role(role.Button).HasClass("mdc-button").Ancestor(main)
	return uiauto.NamedAction("close 'Crop & rotate'",
		g.ui.LeftClick(closeButton),
	)
}

// SetRatio returns a function that sets image ratio at a specific ratio.
func (g *Gallery) SetRatio(r Ratio) uiauto.Action {
	ratioButton := nodewith.Name(string(r)).Role(role.RadioButton).Ancestor(RootFinder)
	return uiauto.NamedAction(fmt.Sprintf("set ratio to %s", r),
		g.ui.LeftClick(ratioButton),
	)
}

// SetLightingFilters returns a function that sets the specific slider of
// lighting filters to the desired direction for a specific count.
func (g *Gallery) SetLightingFilters(kb *input.KeyboardEventWriter, s Slider, direction string, count int) uiauto.Action {
	ui := g.ui
	lightingFiltersButton := nodewith.Name("Lighting filters").Role(role.ToggleButton).Ancestor(RootFinder)
	expectedSlider := nodewith.Name(string(s)).Role(role.Slider).Ancestor(RootFinder)
	set := uiauto.Combine("set to "+direction,
		kb.AccelAction(direction),
		uiauto.Sleep(500*time.Millisecond),
	)
	actionName := fmt.Sprintf("set %s to %s for %v times", s, direction, count)
	return uiauto.NamedCombine(actionName,
		ui.LeftClickUntil(lightingFiltersButton,
			ui.WithTimeout(5*time.Second).WaitUntilExists(expectedSlider),
		),
		ui.FocusAndWait(expectedSlider),
		uiauto.Repeat(count, set),
	)
}

// DrawOnImage returns a function that draws a pattern on the image through
// points.
func (g *Gallery) DrawOnImage(points []coords.Point) uiauto.Action {
	ui := g.ui
	drawButton := nodewith.Name("Draw").Role(role.ToggleButton).Ancestor(RootFinder)
	sizeHeading := nodewith.Name("Size").Role(role.Heading).Ancestor(RootFinder)
	return ui.Retry(3, uiauto.NamedCombine("draw",
		uiauto.IfSuccessThen(ui.Gone(sizeHeading),
			ui.LeftClickUntil(drawButton,
				ui.WithTimeout(3*time.Second).WaitUntilExists(sizeHeading))),
		g.WaitUntilSpinnerGone(),
		g.draw(imageCanvas, points),
		g.Done(),
		g.WaitUntilSpinnerGone(),
	))
}
