// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package effectshtml contains common data types and UI libraries for fake HTML.
package effectshtml

import (
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
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

var videoPauseButton = nodewith.Name("pause").Role(role.Button).Ancestor(VideoNode)

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
	var bounds1 image.Rectangle = img1.Bounds()
	var bounds2 image.Rectangle = img2.Bounds()
	var width = int(math.Min(float64(bounds1.Max.X-bounds1.Min.X+1), float64(bounds2.Max.X-bounds2.Min.X+1)))
	var height = int(math.Min(float64(bounds1.Max.Y-bounds1.Min.Y+1), float64(bounds2.Max.Y-bounds2.Min.Y+1)))

	numOfPixel := float64(width) * float64(height)
	numOfSamePixel := 0.0
	numOfDiffPixel := 0.0
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			r1, g1, b1, _ := img1.At(x+bounds1.Min.X, y+bounds1.Min.Y).RGBA()
			r2, g2, b2, _ := img2.At(x+bounds2.Min.X, y+bounds2.Min.Y).RGBA()
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
	if img == nil {
		s.Errorf("Can't save a nil as image to: %s", fileName)
		return
	}

	dir := filepath.Join(s.OutDir(), faillogDir)
	if err := os.MkdirAll(dir, 0777); err != nil {
		s.Errorf("Can't create path: %s", dir)
		return
	}

	if err := screenshot.SaveImageToFile(img, dir, fileName)(ctx); err != nil {
		s.Errorf("Can't save file: %s", fileName)
	}
}

// OpenURLAndWaitForStreamToReady waits until camera stream is loaded and vcTray is triggered.
func OpenURLAndWaitForStreamToReady(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, url string, vcTray *vctray.VCTray) error {
	if _, err := br.NewTab(ctx, url); err != nil {
		return err
	}

	if _, err := ash.WaitForAnyWindow(ctx, tconn, func(w *ash.Window) bool {
		return strings.Contains(w.Title, PageTitle) && w.IsVisible && !w.IsAnimating
	}); err != nil {
		return err
	}

	if err := uiauto.New(tconn).WaitUntilExists(videoPauseButton)(ctx); err != nil {
		return errors.Wrap(err, "Fail to wait for camera stream to load")
	}

	if err := vcTray.WaitUntilExists(ctx); err != nil {
		return errors.Wrap(err, "Fail to wait for VcTray to show")
	}

	if err := vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse)(ctx); err != nil {
		return errors.Wrap(err, "Fail to wait for VcTray to show camera is in use")
	}

	return nil
}

// CloseURLAndWaitForWindowGone close the tab with the url and wait until vcTray disappears.
func CloseURLAndWaitForWindowGone(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, url string, vcTray *vctray.VCTray) error {
	if err := br.CloseWithURL(ctx, url); err != nil {
		return errors.Wrap(err, "Fail to close url")
	}

	if err := ash.WaitForAllWindowCondition(ctx, tconn, func(w *ash.Window) bool { return !strings.Contains(w.Title, PageTitle) }); err != nil {
		return err
	}

	if err := vcTray.WaitUntilGone(ctx); err != nil {
		return errors.Wrap(err, "Fail to wait for VcTray to disappear")
	}

	return nil
}
