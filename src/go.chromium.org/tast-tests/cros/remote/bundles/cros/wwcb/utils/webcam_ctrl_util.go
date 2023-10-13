// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"image/color"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// RGBColors are used to guide correspondce between multiple webcams and displays.
	RGBColors = []color.RGBA{
		{255, 0, 0, 255},
		{0, 255, 0, 255},
		{0, 0, 255, 255},
	}
)

// OpenGalleryOnDisplay opens a file in the Gallery to the specific display.
func OpenGalleryOnDisplay(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, displaySvc wwcb.DisplayServiceClient, dispIndex int, fileName string) (string, error) {
	galleryTitle, err := OpenFileInGallery(ctx, appsSvc, uiautoSvc, fileName)
	if err != nil {
		return "", errors.Wrap(err, "open file in the gallery")
	}

	// Since there may be multiple monitors, the switch needs to be retried.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: int32(dispIndex), WindowTitle: galleryTitle}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		return "", errors.Wrapf(err, "failed to switch Gallery window to display %d", dispIndex)
	}

	// Maximize the gallery to let the webcam capture more accurate pictures.
	if _, err := displaySvc.EnsureSetWindowState(ctx, &wwcb.QueryRequest{WindowTitle: galleryTitle, WindowState: wwcb.WindowStateType_WINDOW_STATE_MAXIMIZED}); err != nil {
		return "", errors.Wrapf(err, "failed to set the window state of the %s as %s", galleryTitle, wwcb.WindowStateType_WINDOW_STATE_MAXIMIZED.String())
	}

	return galleryTitle, nil
}

// OpenRGBImageOnDisplays opens the image on the multiple displays in red, green and blue order.
func OpenRGBImageOnDisplays(ctx context.Context, fs *dutfs.Client, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, displaySvc wwcb.DisplayServiceClient, dispNum int) error {
	for dispIndex := 0; dispIndex < dispNum; dispIndex++ {
		// Write image into the MyFiles folder.
		image := GenerateImage(3840, 2160, RGBColors[dispIndex])
		imageFilename := strconv.Itoa(dispIndex) + ".jpg"
		imageFilepath := filepath.Join(MyFilesPath, imageFilename)
		if err := WriteImageOnDUT(ctx, fs, image, imageFilepath); err != nil {
			return errors.Wrap(err, "failed to write image into MyFiles")
		}

		windowTitle, err := OpenGalleryOnDisplay(ctx, appsSvc, uiautoSvc, displaySvc, dispIndex, imageFilename)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s on the display %d", windowTitle, dispIndex)
		}
	}
	return nil
}

// PairWebcamToDisplay pairs between multiple displays and webcams using the RGB color image displayed on the screen.
func PairWebcamToDisplay(ctx context.Context, s *testing.State, displayIDs []string) error {
	if len(displayIDs) > len(webcamOnline) {
		return errors.Errorf("Expect the number of online webcams is higher than the number of displays; webcams: %d, displays: %d", len(webcamOnline), len(displayIDs))
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		// Let each webcam capture the image and store the average pixel.
		camPxl := make(map[string]Pixel)
		for _, webcam := range webcamOnline {
			avgPixel, err := getAvgPixelFromWebcam(ctx, s, webcam, "mapping")
			if err != nil {
				return errors.Wrap(err, "get average pixel from webcam")
			}
			camPxl[webcam] = filterColorPixelValue(avgPixel)
		}

		mappingColors := [3]Pixel{redColor, greenColor, blueColor}
		alreadyMappingCamera := make(map[string]bool)
		for dispIndex, dispID := range displayIDs {
			maxScore := -1
			mappingPort := ""
			mappingColor := mappingColors[dispIndex]

			testing.ContextLogf(ctx, "==== Display %d: %s ====", dispIndex, dispID)
			for cam, pxl := range camPxl {
				// Skip it if already mapped.
				if alreadyMappingCamera[cam] {
					continue
				}

				// Use display index to find which RGB values in pixel use as the comparison reference.
				rgbScore := []int{pxl.R, pxl.G, pxl.B}
				imgScore := rgbScore[dispIndex]
				testing.ContextLogf(ctx, "Webcam: %s, Pixel: %v, Score: %d", cam, pxl, imgScore)

				if imgScore > maxScore {
					mappingPort = cam
					maxScore = imgScore
				}
			}

			// Check the color image showing on the display is good enough.
			if maxScore < webcamMappingLimitScore {
				p := camPxl[mappingPort]
				grayScore := scalarScore(p, grayColor)
				expectColorScore := scalarScore(p, mappingColor)

				if int(expectColorScore) < webcamMappingLimitScore && grayScore > expectColorScore {
					return errors.Errorf("'Display: %d, %s' is abnormal. Please check if the display is on. (GrayScore (%f) is higher than ColorScore (%f))", dispIndex, dispID, grayScore, expectColorScore)
				}
			}

			screenToCamera[dispID] = mappingPort
			alreadyMappingCamera[mappingPort] = true
			testing.ContextLogf(ctx, "Mapping '%s' to 'Display %d, %s' within the score is %d", mappingPort, dispIndex, dispID, maxScore)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second})
}
