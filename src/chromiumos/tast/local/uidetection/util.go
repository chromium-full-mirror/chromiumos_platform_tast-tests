// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/coords"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
)

const (
	outputDir         = "uidetection"
	screenshotFile    = "uidetection_screenshot.png"
	oldScreenshotFile = "old_uidetection_screenshot.png"
)

// readImage reads a PNG image and returns it in []byte.
func readImage(imgFile string) ([]byte, error) {
	// Read an image in PNG format.
	f, err := os.Open(imgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open the image: %q", imgFile)
	}
	defer f.Close()

	imgPNG, _, err := image.Decode(f)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode the image")
	}

	imgBuf := new(bytes.Buffer)
	if err := png.Encode(imgBuf, imgPNG); err != nil {
		return nil, errors.Wrap(err, "failed to write the PNG image into byte buffer")
	}

	return imgBuf.Bytes(), nil
}

func crop(img image.Image, boundingBox coords.Rect) (image.Image, error) {
	bounds := img.Bounds().Intersect(image.Rect(boundingBox.Left, boundingBox.Top, boundingBox.Right(), boundingBox.Bottom()))
	cropped := image.NewRGBA(bounds)
	draw.Draw(cropped, bounds, img, bounds.Min, draw.Src)
	if cropped.Bounds().Empty() {
		return nil, errors.New(ErrEmptyBoundingBox)
	}
	return cropped, nil
}

// saveDebugImages saves two images for debugging:
//
// 1. The screenshot that was sent to ACUITI.
//
// 2. The screenshot with detection outlines drawn over the image.
func saveDebugImages(ctx context.Context, imagePng []byte, image image.Image, locations []Location, desc string) {
	desc = strings.TrimSuffix(desc, ".png")
	desc = strings.ReplaceAll(desc, "/", "")
	filename := time.Now().UTC().Format("2006-01-02T15:04:05.000000Z") + "-" + desc
	debugFilename := filename + ".png"
	if err := saveBytesImageToOutput(ctx, imagePng, debugFilename); err != nil {
		testing.ContextLogf(ctx, "INFO: couldn't save debug screenshot to %s: %s", debugFilename, err)
	}

	outlinesFilename := filename + "-" + strconv.Itoa(len(locations)) + "_detection_outlines.png"
	if err := saveDetectionOutlineImage(ctx, image, locations, outlinesFilename); err != nil {
		testing.ContextLogf(ctx, "INFO: couldn't save debug screenshot with detection outlines to %s: %s", outlinesFilename, err)
	}
}

// saveDetectionOutlineImage saves a screenshot with rectangles drawn around the
// detection regions.
func saveDetectionOutlineImage(ctx context.Context, img image.Image, locations []Location, filename string) error {
	if len(locations) == 0 {
		return saveImageToOutput(ctx, img, filename)
	}

	// Cast because image.Image is readonly.
	imgRGBA, ok := img.(*image.RGBA)
	if !ok {
		return errors.New("failed to cast image to RGBA")
	}

	color := color.RGBA{255, 0, 255, 255} // pink
	var detectionImg draw.Image
	for _, loc := range locations {
		// Draw a rectangle 1px outside the detection region, bounded to the
		// size of the image.
		rec := loc.WithInset(-2, -2)

		bounds := img.Bounds()
		boundsRec := coords.NewRectLTRB(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y)
		rec = rec.Intersection(boundsRec)

		x1, y1, x2, y2 := rec.TopLeft().X, rec.TopLeft().Y, rec.BottomRight().X, rec.BottomRight().Y
		detectionImg = drawRectangle(imgRGBA, color, x1, y1, x2, y2)
	}

	return saveImageToOutput(ctx, detectionImg, filename)
}

// drawRectangle draws an unfilled rectangle on an image.
func drawRectangle(img draw.Image, color color.Color, x1, y1, x2, y2 int) draw.Image {
	for i := x1; i < x2; i++ {
		img.Set(i, y1, color)
		img.Set(i, y2, color)
	}

	for i := y1; i <= y2; i++ {
		img.Set(x1, i, color)
		img.Set(x2, i, color)
	}
	return img
}

// takeScreenshot takes a screenshot in PNG format.
func takeScreenshot(ctx context.Context, tconn *chrome.TestConn, boundingBox coords.Rect, scaleFactor float64, disableMaskDynamics bool) (image.Image, error) {
	uncropped, err := screenshot.CaptureChromeImageWithTestAPI(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to take the screenshot")
	}
	if !disableMaskDynamics {
		uncropped, err = maskDynamics(ctx, tconn, uncropped, scaleFactor)
		if err != nil {
			return nil, errors.Wrap(err, "failed to mask dynamic elements in screenshot")
		}
	}

	cropped, err := crop(uncropped, boundingBox)
	if err != nil {
		return nil, err
	}

	return cropped, nil
}

// takeStableScreenshot takes a stable screenshot in PNG format that doesn't
// change between two polls.
func takeStableScreenshot(ctx context.Context, tconn *chrome.TestConn, pollOpts testing.PollOptions, boundingBox coords.Rect, scaleFactor float64, disableMaskDynamics bool) (image.Image, error) {
	var currentScreen image.Image
	var lastScreen image.Image
	start := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		lastScreen = currentScreen
		uncropped, err := screenshot.CaptureChromeImageWithTestAPI(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to take immediate screenshot")
		}
		if !disableMaskDynamics {
			uncropped, err = maskDynamics(ctx, tconn, uncropped, scaleFactor)
			if err != nil {
				return errors.Wrap(err, "failed to mask dynamic elements in screenshot")
			}
		}
		currentScreen, err = crop(uncropped, boundingBox)
		if err != nil {
			return err
		}
		if err = equal(currentScreen, lastScreen); err != nil {
			return errors.Wrapf(err, "screen has not stopped changing after %s, perhaps increase timeout or use immediate-screenshot strategy", time.Since(start))
		}
		return nil
	}, &pollOpts); err != nil {
		// Save two screenshots to output dir in case of error.
		if err := saveImageToOutput(ctx, currentScreen, screenshotFile); err != nil {
			testing.ContextLogf(ctx, "INFO: couldn't save the screenshot to %s for debugging takeStableScreenshot: %s", screenshotFile, err)
		}
		if err := saveImageToOutput(ctx, lastScreen, oldScreenshotFile); err != nil {
			testing.ContextLogf(ctx, "INFO: couldn't save the old screenshot to %s for debugging takeStableScreenshot: %s", oldScreenshotFile, err)
		}
		return nil, errors.Wrap(err, "failed to take stable screenshot")
	}
	return currentScreen, nil
}

func maskDynamics(ctx context.Context, tconn *chrome.TestConn, screenshot image.Image, scaleFactor float64) (image.Image, error) {
	maskedScreenshot := image.NewRGBA(screenshot.Bounds())
	draw.Draw(maskedScreenshot, screenshot.Bounds(), screenshot, image.Point{}, draw.Src)
	var err error

	for _, node := range []*nodewith.Finder{
		nodewith.HasClass("TimeView").Ancestor(nodewith.HasClass("DateTray")).Onscreen(),
		nodewith.HasClass("TimeView").Ancestor(nodewith.HasClass("UnifiedSystemTray")).Onscreen(),
		nodewith.HasClass("NetworkTrayView").Ancestor(nodewith.HasClass("UnifiedSystemTray")).Onscreen(),
		nodewith.HasClass("PowerTrayView").Ancestor(nodewith.HasClass("UnifiedSystemTray")).Onscreen(),
	} {
		maskedScreenshot, err = maskA11yNode(ctx, tconn, maskedScreenshot, node, scaleFactor)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to mask node: %s", node.Pretty())
		}
	}
	return maskedScreenshot, nil
}

func maskA11yNode(ctx context.Context, tconn *chrome.TestConn, screenshot *image.RGBA, node *nodewith.Finder, scaleFactor float64) (*image.RGBA, error) {
	coordinatesDP, err := uiauto.New(tconn).ImmediateLocation(ctx, node)
	if err != nil {
		// Do not mask if the node is not onscreen.
		if nodewith.IsNodeNotFoundErr(err) {
			return screenshot, nil
		}
		return nil, err
	}
	coordinatesPX := coords.ConvertBoundsFromDPToPX((*coordinatesDP), scaleFactor)
	bounds := image.Rectangle{
		Min: image.Point(coordinatesPX.TopLeft()),
		Max: image.Point(coordinatesPX.BottomRight()),
	}
	draw.Draw(screenshot, bounds, &image.Uniform{color.Black}, image.Point{}, draw.Src)
	return screenshot, nil
}

// encodePNG converts an image.Image to a PNG.
func encodePNG(img image.Image) ([]byte, error) {
	imgBuf := new(bytes.Buffer)
	if err := png.Encode(imgBuf, img); err != nil {
		return nil, errors.Wrap(err, "failed to write the PNG image into byte buffer")
	}
	return imgBuf.Bytes(), nil
}

// equal returns error if two images are not the same.
// The error message contains information about how they differ.
func equal(imgA, imgB image.Image) error {
	// Two images are considered equal if the colors at every pixel is the same.
	// Two nil images are also considered equal.
	if imgA == nil && imgB == nil {
		return nil
	} else if imgA == nil || imgB == nil {
		return errors.New("one image is nil while the other is not")
	}
	if imgA.Bounds() != imgB.Bounds() {
		return errors.New("two images are in different sizes")
	}
	for y := imgA.Bounds().Min.Y; y < imgA.Bounds().Max.Y; y++ {
		for x := imgA.Bounds().Min.X; x < imgA.Bounds().Max.X; x++ {
			if imgA.At(x, y) != imgB.At(x, y) {
				return errors.Errorf(
					"Screen has changed since the last screenshot. Images %s and %s differ at (%d, %d)",
					oldScreenshotFile,
					screenshotFile,
					x-imgA.Bounds().Min.X,
					y-imgA.Bounds().Min.Y)
			}
		}
	}
	return nil
}

// findOutputPath returns the path to the uidetection output directory and creates
// it if it does not already exist.
func findOutputPath(ctx context.Context) (string, error) {
	outputPath, ok := testing.ContextOutDir(ctx)
	if !ok {
		return "", errors.New("failed to get the testing output directory")
	}
	outputPath = filepath.Join(outputPath, outputDir)

	// Create the output directory if it does not exist.
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return "", errors.Wrap(err, "failed to create the testing output directory")
	}
	return outputPath, nil
}

// saveImageToOutput saves an image in image.Image format to the testing output
// dir that will be uploaded to the test log folder.
func saveImageToOutput(ctx context.Context, img image.Image, filename string) error {
	outputPath, err := findOutputPath(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get the output directory to save file %s", filename)
	}
	if err := saveImage(img, filepath.Join(outputPath, filename)); err != nil {
		return errors.Wrapf(err, "failed to save file %s", filename)
	}
	return nil
}

// saveBytesImageToOutput saves an image in []byte format to the testing output
// dir that will be uploaded to the test log folder.
func saveBytesImageToOutput(ctx context.Context, img []byte, filename string) error {
	outputPath, err := findOutputPath(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get the output directory to save file %s", filename)
	}
	if err := ioutil.WriteFile((filepath.Join(outputPath, filename)), img, 0644); err != nil {
		return errors.Wrapf(err, "failed to save file %s", filename)
	}
	return nil
}

// saveImage saves an image in image.Image format to a specified path.
func saveImage(img image.Image, path string) error {
	if img == nil {
		return errors.New("image is nil")
	}

	f, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "failed to create the PNG file")
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return errors.Wrap(err, "failed to write the PNG image into file")
	}
	return nil
}
