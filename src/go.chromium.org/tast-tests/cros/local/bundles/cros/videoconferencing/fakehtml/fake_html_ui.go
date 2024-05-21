// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakehtml contains common data types and UI libraries for fake HTML.
package fakehtml

import (
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/screenshot"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	faillogDir = "faillog"

	// BeforeEffectsImageName is the filename to save the screenshot before camera effects.
	BeforeEffectsImageName = "before_effects.png"
	// AfterEffectsImageName is the filename to save the screenshot after camera effects.
	AfterEffectsImageName = "after_effects.png"
)

// UIObject represents the fake HTML page UI.
type UIObject struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

const (
	// PageTitle is the page title.
	PageTitle = "Simple Meeting"
	// PageURL is the page base URL.
	PageURL = "/effects_video_script.html?resolution="
)

var rootWebArea = nodewith.Role(role.RootWebArea).Name(PageTitle)

// VideoNode is the node showing camera.
var VideoNode = nodewith.Role(role.Video)

// NewUI creates a new instance to represent the fake HTML page UI.
func NewUI(tconn *chrome.TestConn) UIObject {
	return UIObject{
		tconn: tconn,
		ui:    uiauto.New(tconn),
	}
}

// MayBeAllowCameraAccess allows Camera access if the prompt is shown up.
func (htmlUI UIObject) MayBeAllowCameraAccess(ctx context.Context) error {
	return prompts.ClearPotentialPrompts(htmlUI.tconn, 3*time.Second, prompts.AllowAVPermissionPrompt)(ctx)
}

// EnterFullScreen double clicks video frame and waits for the window to be full screen.
func (htmlUI UIObject) EnterFullScreen(ctx context.Context) error {
	return htmlUI.ui.RetryUntil(
		htmlUI.ui.DoubleClick(VideoNode),
		ash.WaitForFullscreenConditionWithTitle(htmlUI.tconn, PageTitle, true, 5*time.Second),
	)(ctx)
}

// GrabVideoArea takes a screen shot of the VideoNode area.
func GrabVideoArea(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, ui *uiauto.Context) (image.Image, error) {
	if err := ui.WaitUntilExists(VideoNode)(ctx); err != nil {
		return nil, errors.Wrap(err, "Fail to find camera feed")
	}

	// Get location of the camera element.
	loc, err := ui.Location(ctx, VideoNode)
	if err != nil {
		return nil, errors.Wrap(err, "Fail to get video node location")
	}

	// deviceScaleFactor is required to convert location to pixels.
	deviceScaleFactor, err := display.GetDeviceScaleFactor(ctx, tconn,
		func(info *display.Info) bool {
			return info.IsPrimary
		})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get primary display scale factor")
	}

	videoRect := coords.ConvertBoundsFromDPToPX(*loc, deviceScaleFactor)

	// Take a screen shot before background replace is applied.
	image, err := screenshot.GrabAndCropScreenshot(ctx, cr, videoRect)
	if err != nil {
		return nil, errors.Wrap(err, "Fail to grab camera screen shot")
	}

	return image, nil
}

// ImageDiff returns whether two images have enough portion changed and unchanged.
func ImageDiff(img1, img2 image.Image, threshold float64) (float64, float64) {
	var bounds image.Rectangle = img1.Bounds()
	numOfPixel := float64(bounds.Max.X-bounds.Min.X+1) * float64(bounds.Max.Y-bounds.Min.Y+1)
	numOfSamePixel := 0.0
	numOfDiffPixel := 0.0
	for x := bounds.Min.X; x <= bounds.Max.X; x++ {
		for y := bounds.Min.Y; y <= bounds.Max.Y; y++ {
			r1, g1, b1, _ := img1.At(x, y).RGBA()
			r2, g2, b2, _ := img2.At(x, y).RGBA()
			dis := math.Abs(float64(r1)-float64(r2)) + math.Abs(float64(g1)-float64(g2)) + math.Abs(float64(b1)-float64(b2))
			if dis <= threshold {
				numOfSamePixel += 1.0
			} else {
				numOfDiffPixel += 1.0
			}
		}
	}

	return numOfSamePixel / numOfPixel, numOfDiffPixel / numOfPixel
}

// SaveImageToFaillog saves image to faillog for debugging.
func SaveImageToFaillog(ctx context.Context, s *testing.State, img image.Image, fileName string) {
	dir := filepath.Join(s.OutDir(), faillogDir)
	if err := os.MkdirAll(dir, 0777); err != nil {
		s.Errorf("Can't create path: %s", dir)
	}

	if err := screenshot.SaveImageToFile(img, dir, fileName)(ctx); err != nil {
		s.Errorf("Can't save file: %s", fileName)
	}
}
