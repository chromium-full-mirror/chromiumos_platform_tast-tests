// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util contains functions that help with common actions for Mantis tast tests.
package util

import (
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/updateengine"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

var (
	backlightDropTarget = nodewith.HasClass("backlight-drop-target").Ancestor(galleryapp.RootFinder)
	// ImageCanvas is the node finder of the image opened in the Gallery app.
	ImageCanvas = nodewith.Role(role.Image).Ancestor(backlightDropTarget).First()
)

// DownloadAndOpenFileInGallery is used to download an image and then open the image in Gallery app.
func DownloadAndOpenFileInGallery(ctx context.Context, cr *chrome.Chrome, testFileDataPath, testFile string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}

	ui := uiauto.New(tconn)
	localFileLocation := filepath.Join(downloadsPath, testFile)
	if err := ui.WithInterval(time.Second).Retry(10, func(context.Context) error {
		return fsutil.CopyFile(testFileDataPath, localFileLocation)
	})(ctx); err != nil {
		return errors.Wrapf(err, "failed to copy the test image to %s", localFileLocation)
	}

	return OpenGalleryFromDownload(ctx, ui, tconn, testFile)
}

// OpenGalleryFromDownload opens the gallery app by clicking on a file in Downloads folder.
func OpenGalleryFromDownload(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, testFile string) error {
	// SWA installation is not guaranteed during startup.
	// Using this wait to check installation finished.
	testing.ContextLog(ctx, "Wait for Gallery to be installed")
	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Gallery.ID, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for Gallery to be installed")
	}

	// Open the Files App.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "launching the Files App failed")
	}

	if err := uiauto.Combine("open Downloads folder and double click file to launch Gallery",
		files.OpenDownloads(),
		files.WithTimeout(30*time.Second).WaitForFile(testFile),
		files.OpenFile(testFile),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open file in Downloads")
	}

	testing.ContextLog(ctx, "Wait for Gallery shown in shelf")
	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		return errors.Wrap(err, "failed to check Gallery in shelf")
	}

	testing.ContextLog(ctx, "Wait for Gallery app rendering")
	fileElement := nodewith.Role(role.Window).Name(testFile).Ancestor(galleryapp.RootFinder).First()
	if err := ui.WithInterval(time.Second).WaitUntilExists(fileElement)(ctx); err != nil {
		return errors.Wrap(err, "failed to render Gallery")
	}

	files.Close(ctx)

	return nil
}

// EnsureDLCInstalled ensures a DLC package is installed.
func EnsureDLCInstalled(ctx context.Context, dlcID string) error {
	// Ensure that the update engine service is ready to receive DLC install request from DLC service.
	if err := upstart.EnsureJobRunning(ctx, updateengine.JobName); err != nil {
		return errors.Wrapf(err, "failed to ensure %s running", updateengine.JobName)
	}
	if bus, err := dbusutil.SystemBus(); err != nil {
		return errors.Wrap(err, "failed to connect to the message bus")
	} else if err := dbusutil.WaitForService(ctx, bus, updateengine.ServiceName); err != nil {
		return errors.Wrapf(err, "failed to wait for D-Bus service %s", updateengine.ServiceName)
	}

	// Check dlcservice is up and running.
	if err := upstart.EnsureJobRunning(ctx, dlc.JobName); err != nil {
		return errors.Wrapf(err, "failed to ensure %s running", dlc.JobName)
	}

	// Install DLC.
	if err := dlc.Install(ctx, dlcID, ""); err != nil {
		return errors.Wrap(err, "failed to install DLC")
	}

	return nil
}

// DrawOnImage draws a line on image in Gallery app.
func DrawOnImage(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	canvasBounds, err := ui.ImmediateLocation(ctx, ImageCanvas)
	if err != nil {
		return errors.Wrap(err, "failed to get the canvas location")
	}

	startLocation := coords.NewPoint(
		canvasBounds.CenterX(),
		canvasBounds.CenterY(),
	)

	endLocation := coords.NewPoint(
		canvasBounds.CenterX()+50,
		canvasBounds.CenterY()+50,
	)

	return DrawOnImageWithLocation(ctx, tconn, ui, startLocation, endLocation)
}

// DrawOnImageWithLocation Draws a line on an image in the Gallery app from a specified start to end location.
func DrawOnImageWithLocation(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, start, end coords.Point) error {
	if err := mouse.Move(tconn, start, 200*time.Millisecond)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	if err := mouse.Press(tconn, mouse.LeftButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to press mouse")
	}

	if err := mouse.Move(tconn, end, 200*time.Millisecond)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	if err := mouse.Release(tconn, mouse.LeftButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to release mouse")
	}

	return nil
}

// WaitForSpinner waits for spinner until it is gone.
func WaitForSpinner(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	spinner := nodewith.HasClass("mdc-circular-progress__spinner-layer").Ancestor(galleryapp.RootFinder).First()
	if err := uiauto.Combine("Waiting for spinner",
		ui.WithTimeout(3*time.Second).WaitUntilExists(spinner),
		ui.WithTimeout(1*time.Minute).WaitUntilGone(spinner))(ctx); err != nil {
		return errors.Wrap(err, "error while waiting for spinner")
	}

	return nil
}

// WaitForProgressBar waits for progress bar until it is gone.
func WaitForProgressBar(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	progressBar := nodewith.Role(role.ProgressIndicator).Ancestor(galleryapp.RootFinder).First()
	if err := uiauto.Combine("Waiting for progress bar",
		ui.WithTimeout(3*time.Second).WaitUntilExists(progressBar),
		// DLC download might take up to 20 minutes.
		ui.WithTimeout(20*time.Minute).WaitUntilGone(progressBar))(ctx); err != nil {
		return errors.Wrap(err, "error while waiting for progress bar")
	}

	return nil
}

// LeftClickButton clicks on a button, will return an error if the button doesn't exist.
// TODO(b/383666179): Remove and replace this function with ui.DoDefault.
func LeftClickButton(ctx context.Context, ui *uiauto.Context, button *nodewith.Finder) error {
	return uiauto.Combine("Left click on a button",
		ui.WithTimeout(time.Minute).WaitUntilExists(button),
		ui.LeftClick(button))(ctx)
}

// GrabCanvasArea takes a screenshot of the canvas area.
func GrabCanvasArea(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, ui *uiauto.Context) (image.Image, error) {
	if err := ui.WaitUntilExists(ImageCanvas)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find image canvas")
	}

	// Get location of the image canvas.
	loc, err := ui.Location(ctx, ImageCanvas)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get image canvas location")
	}

	// deviceScaleFactor is required to convert location to pixels.
	deviceScaleFactor, err := display.GetDeviceScaleFactor(ctx, tconn,
		func(info *display.Info) bool {
			return info.IsPrimary
		})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get primary display scale factor")
	}

	canvasRect := coords.ConvertBoundsFromDPToPX(*loc, deviceScaleFactor)

	image, err := screenshot.GrabAndCropScreenshot(ctx, cr, canvasRect)
	if err != nil {
		return nil, errors.Wrap(err, "failed to grab screenshot")
	}

	return image, nil
}

// ImageDiff returns the pixel difference between two images.
func ImageDiff(img1, img2 image.Image) float64 {
	var bounds1 image.Rectangle = img1.Bounds()
	var bounds2 image.Rectangle = img2.Bounds()
	var width = int(math.Min(float64(bounds1.Max.X-bounds1.Min.X+1), float64(bounds2.Max.X-bounds2.Min.X+1)))
	var height = int(math.Min(float64(bounds1.Max.Y-bounds1.Min.Y+1), float64(bounds2.Max.Y-bounds2.Min.Y+1)))

	numOfDiffPixel := 0.0
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			r1, g1, b1, _ := img1.At(x+bounds1.Min.X, y+bounds1.Min.Y).RGBA()
			r2, g2, b2, _ := img2.At(x+bounds2.Min.X, y+bounds2.Min.Y).RGBA()
			dis := math.Abs(float64(r1)-float64(r2)) + math.Abs(float64(g1)-float64(g2)) + math.Abs(float64(b1)-float64(b2))
			if dis > 0 {
				numOfDiffPixel += 1.0
			}
		}
	}

	return numOfDiffPixel
}

// CloseGallery closes the gallery app.
func CloseGallery(ctx context.Context, tconn *chrome.TestConn) error {
	if err := apps.Close(ctx, tconn, apps.Gallery.ID); err != nil {
		return errors.Wrap(err, "failed to close gallery")
	}
	if err := ash.WaitForAppClosed(ctx, tconn, apps.Gallery.ID); err != nil {
		return errors.Wrap(err, "failed waiting for gallery to be closed")
	}
	return nil
}

// FetchImage fetches image from the provided file path.
func FetchImage(filePath string) (image.Image, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file %s", filePath)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode file")
	}

	return img, nil
}
