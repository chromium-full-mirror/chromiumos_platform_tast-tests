// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package privacyhubutil contains utility functions for privacy hub tests.
package privacyhubutil

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SaveImage saves a given image.Image as png.
func SaveImage(outDir, name string, img image.Image) error {
	f, err := os.Create(fmt.Sprintf("%s/%s", outDir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// IsImageBlack checks whether the given image.Image is black.
func IsImageBlack(img image.Image) (bool, error) {
	if img == nil {
		return false, errors.New("Image is nil")
	}

	var bounds image.Rectangle = img.Bounds()

	for x := bounds.Min.X; x <= bounds.Max.X; x++ {
		for y := bounds.Min.Y; y <= bounds.Max.Y; y++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r+g+b != 0 {
				return false, nil
			}
		}
	}

	return true, nil
}

func launchCameraAppFromHomeMenu(ctx context.Context, tconn *browser.TestConn) {
	ui := uiauto.New(tconn)
	homeButton := nodewith.Role("button").Name("Launcher").ClassName("ash/HomeButton")
	cameraAppButton := nodewith.Role("button").Name("Camera").ClassName("AppListItemView").First()

	if err := ui.LeftClick(homeButton)(ctx); err != nil {
		errors.Wrap(err, "failed to left click")
	}
	if err := ui.LeftClick(cameraAppButton)(ctx); err != nil {
		errors.Wrap(err, "failed to right click the Camera App")
	}
}

func getRectPXForUIElement(ctx context.Context, cr *chrome.Chrome, tconn *browser.TestConn,
	finderForUIElement *nodewith.Finder) (*coords.Rect, error) {
	ui := uiauto.New(tconn)

	// Get bounds in DP for the UI element.
	frameLocDP, err := ui.Location(ctx, finderForUIElement)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain camera app Location")
	}

	// Convert bounds from DP to PX.
	displayInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the primary display info")
	}
	displayMode, err := displayInfo.GetSelectedMode()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the selected display mode of the primary display")
	}
	rect := coords.ConvertBoundsFromDPToPX(*frameLocDP, displayMode.DeviceScaleFactor)
	return &rect, nil
}

// LaunchCameraAndTakeScreenshot starts the Camera app and
// takes a screenshot of content (cropped, without UI elements).
func LaunchCameraAndTakeScreenshot(ctx context.Context, cr *chrome.Chrome,
	tconn *browser.TestConn, s *testing.State) (image.Image, error) {

	launchCameraAppFromHomeMenu(ctx, tconn)

	ui := uiauto.New(tconn)

	// Wait till the camera frame will appear.
	cameraBrowserFrame := nodewith.Role("window").ClassName("BrowserFrame").Name("Camera")
	cameraFrame := nodewith.ClassName("RenderWidgetHostViewAura").FinalAncestor(cameraBrowserFrame)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(cameraBrowserFrame)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find camera view during a defined timeout")
	}

	// Get rect in px of camera app.
	rectPX, err := getRectPXForUIElement(ctx, cr, tconn, cameraFrame)

	// We need to have a subRectPX, as the rectPX holds
	// the entire viewing area of the camera app
	// (with buttons & UI elements we shall not capture).
	var subRectPX = rectPX.WithScaleAboutCenter(0.70, 0.70)
	subRectPX.Top = rectPX.Top
	subRectPX = subRectPX.WithScaleAboutCenter(0.90, 0.90)

	// Polling at most 10 sec till camera will start showing a stream
	// (as the camera takes 1-2 sec to start capturing
	// the real picture, or will keep showing black if camera is off).
	if err = testing.Poll(ctx, func(ctx context.Context) error {

		img, err := screenshot.GrabAndCropScreenshot(ctx, cr, subRectPX)
		if err != nil {
			return testing.PollBreak(err)
		}

		isStreamBlack, err := IsImageBlack(img)
		if err != nil {
			return testing.PollBreak(err)
		}

		if isStreamBlack {
			// Return error to repeat an another poll.
			return errors.New("the camera is not showing image yet")
		}

		// The camera app shows non-black stream - stop the Pooling (return nil).
		return nil

	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		wasPoolTimeouted := strings.Contains(err.Error(), "context deadline exceeded")
		// Return the err only if it was not a timeout
		// (it's expected that pooling may timeout).
		if !wasPoolTimeouted {
			return nil, errors.Wrap(err, "failed to wait for camera stream")
		}
	}

	// Grab and return the camera stream.
	var sshot image.Image
	sshot, err = screenshot.GrabAndCropScreenshot(ctx, cr, subRectPX)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create camera feed image")
	}
	return sshot, nil
}
