// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"time"

	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type pixel = passport.Pixel

var (
	redColor   = &pixel{R: 240, G: 80, B: 80, A: 255}
	greenColor = &pixel{R: 80, G: 240, B: 80, A: 255}
	blueColor  = &pixel{R: 80, G: 80, B: 240, A: 255}
	grayColor  = &pixel{R: 120, G: 120, B: 120, A: 255}

	// webcamMappingLimitScore is the max allowable "difference" between two pixels
	// to be considered the same.
	webcamMappingLimitScore int32 = 160

	mappingColors    = [3]*pixel{redColor, greenColor, blueColor}
	detectVideoColor = [3]string{"red", "green", "blue"}
)

// Enable Webcam Save Img if value is "on".
var enableWebcamSaveImg = testing.RegisterVarString(
	"api.enableWebcamSaveImg",
	"off",
	"WWCB enable webcam save image",
)

// CameraServiceHelper is a utility class for interacting with a camera service.
type CameraServiceHelper struct {
	service CameraService
}

// NewCameraServiceHelper creates a new helper to wrap a camera service.
func NewCameraServiceHelper(service CameraService) *CameraServiceHelper {
	return &CameraServiceHelper{
		service: service,
	}
}

// InitializeCameras initializes the available cameras.
func (c *CameraServiceHelper) InitializeCameras(ctx context.Context) error {
	resp, err := c.service.GetCameras(ctx, &passport.GetCamerasRequest{})
	if err != nil {
		return errors.Wrap(err, "failed to get available webcams")
	}
	for _, camera := range resp.GetCameras() {
		testing.ContextLogf(ctx, "Found webcam: %q", camera.GetId())
	}
	return nil
}

// VerifyVideo verifies that the camera video contains the set frames in order.
func (c *CameraServiceHelper) VerifyVideo(ctx context.Context, outDir, cameraID string, duration int) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(duration))
	defer cancel()

	detectColorCount := 0
	for timeoutCtx.Err() != nil {
		req := &passport.GetAveragePixelRequest{DeviceId: cameraID}
		resp, err := c.service.GetAveragePixel(ctx, req)
		if err != nil {
			return errors.Wrap(err, "failed to get average pixel color from webcam")
		}

		if err := saveImageIfRequested(ctx, outDir, cameraID, "_verify_", resp.GetFrame()); err != nil {
			return errors.New("webcam with '" + cameraID + "' webcam write file error")
		}

		frameColor := detectColor(resp.GetPixel())
		testing.ContextLog(ctx, "Detect color: "+frameColor)
		testing.ContextLogf(ctx, "Expected color %s", detectVideoColor[detectColorCount])
		if frameColor == detectVideoColor[detectColorCount] {
			testing.ContextLogf(ctx, "%s color detected, incrementing colorCount", frameColor)
			detectColorCount++

			// GoBigSleepLint: successfully identified a frame, wait 1 second for the next frame to appear.
			testing.Sleep(ctx, 1*time.Second)
			if detectColorCount == len(detectVideoColor) {
				break
			}
		} else if detectColorCount > 0 && frameColor != detectVideoColor[detectColorCount-1] {
			// If we detected colors out of order then reset the count and start over.
			detectColorCount = 0
		}
	}

	return nil
}

// GAMLightingValue gets the average pixel "strength" between [0(dark), 255(light)].
func (c *CameraServiceHelper) GAMLightingValue(ctx context.Context, outDir, camera string) (int, error) {
	req := &passport.GetAveragePixelRequest{DeviceId: camera}
	resp, err := c.service.GetAveragePixel(ctx, req)
	if err != nil {
		return 0, err
	}

	if err := saveImageIfRequested(ctx, outDir, camera, "LightingValue", resp.GetFrame()); err != nil {
		return 0, errors.New("webcam with '" + camera + "' webcam write file error")
	}

	pixel := resp.GetPixel()
	return (int(pixel.R+pixel.G+pixel.B) / 3), nil
}

// GAMHotColdValue gets the difference between the average pixel red and blue value.
func (c *CameraServiceHelper) GAMHotColdValue(ctx context.Context, outDir, camera string) (int, error) {
	req := &passport.GetAveragePixelRequest{DeviceId: camera}
	resp, err := c.service.GetAveragePixel(ctx, req)
	if err != nil {
		return 0, err
	}

	if err := saveImageIfRequested(ctx, outDir, camera, "HotColdValue", resp.GetFrame()); err != nil {
		return 0, errors.New("webcam with '" + camera + "' webcam write file error")
	}

	pixel := resp.GetPixel()
	return int(pixel.R - pixel.B), nil
}

// PairWebcamToDisplay matches a camera with a given display.
// It returns a map of display IDs to camera IDs.
func (c *CameraServiceHelper) PairWebcamToDisplay(ctx context.Context, outDir string, displayIDs []string) (map[string]string, error) {
	resp, err := c.service.GetCameras(ctx, &passport.GetCamerasRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get available webcams")
	}

	if len(displayIDs) > len(resp.GetCameras()) {
		return nil, errors.Errorf("Expect the number of online webcams is higher than the number of displays; webcams: %d, displays: %d", len(resp.GetCameras()), len(displayIDs))
	}
	var cameraIDs []string
	for _, camera := range resp.GetCameras() {
		cameraIDs = append(cameraIDs, camera.GetId())
	}

	var displayMappings map[string]string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		displayMappings, err = c.findCameraMatch(ctx, outDir, cameraIDs, displayIDs)
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to match displays to cameras")
	}
	return displayMappings, nil
}

// findCameraMatch finds the matching camera for each display.
func (c *CameraServiceHelper) findCameraMatch(ctx context.Context, outDir string, cameraIDs, displayIDs []string) (map[string]string, error) {
	// Get current image for each camera
	camPxl := make(map[string]*passport.Pixel)
	for _, webcam := range cameraIDs {
		req := &passport.GetAveragePixelRequest{DeviceId: webcam}
		resp, err := c.service.GetAveragePixel(ctx, req)
		if err != nil {
			return nil, errors.Wrap(err, "get average pixel from webcam")
		}

		if err := saveImageIfRequested(ctx, outDir, webcam, "mapping", resp.GetFrame()); err != nil {
			return nil, errors.Wrapf(err, "failed to save frame for webcam: %q", webcam)
		}

		camPxl[webcam] = resp.GetPixel()
	}

	displayMappings := make(map[string]string)
	for dispIndex, dispID := range displayIDs {
		expectedColor := mappingColors[dispIndex]
		testing.ContextLogf(ctx, "==== Display %d: %s ====", dispIndex, dispID)

		// Get the camera that is the closest i.e. has the highest score.
		var maxScore int32 = -1
		mappingPort := ""
		for cam, pxl := range camPxl {
			rgbScore := []int32{pxl.R, pxl.G, pxl.B}
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
			expectColorScore := scalarScore(p, expectedColor)

			if int32(expectColorScore) < webcamMappingLimitScore && grayScore > expectColorScore {
				return nil, errors.Errorf("'Display: %d, %s' is abnormal. Please check if the display is on. (GrayScore (%f) is higher than ColorScore (%f))", dispIndex, dispID, grayScore, expectColorScore)
			}
		}

		// Delete this camera so we don't use it again.
		delete(camPxl, mappingPort)
		testing.ContextLogf(ctx, "Mapping %s to Display %d, %s within the score is %d", mappingPort, dispIndex, dispID, maxScore)
		displayMappings[dispID] = mappingPort
	}
	return displayMappings, nil
}

func saveImageIfRequested(ctx context.Context, outDir, camera, annotation string, image []byte) error {
	if enableWebcamSaveImg.Value() != "on" {
		return nil
	}

	imgFileName := fmt.Sprintf("%s_%s_%s.jpeg", "a", annotation, time.Now().Format("15:04:05"))
	imgFileName = path.Join(outDir, imgFileName)
	return os.WriteFile(imgFileName, image, 0644)
}

// detectColor is for detect color from pixel.
func detectColor(p *pixel) string {
	redScore := int(scalarScore(p, redColor))
	greenScore := int(scalarScore(p, greenColor))
	blueScore := int(scalarScore(p, blueColor))

	if blueScore > redScore && blueScore > greenScore {
		return "blue"
	} else if greenScore > redScore {
		return "green"
	}

	return "red"
}

// distScore is for get the score from two Pixels.
// score more high means two pixels more difference.
func distScore(s1, s2 *pixel) float64 {
	return math.Abs(float64(s1.R-s2.R)) + math.Abs(float64(s1.G-s2.G)) + math.Abs(float64(s1.B-s2.B))
}

// scalarScore is for get the score from two Pixels.
// score more high means two pixels more similar.
func scalarScore(s1, s2 *pixel) float64 {
	return 255 - distScore(s1, s2)/3
}
