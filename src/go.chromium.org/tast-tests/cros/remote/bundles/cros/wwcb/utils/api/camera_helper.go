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
	"path/filepath"
	"time"

	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"
	"go.chromium.org/tast-tests/cros/common/testexec"
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

	expectedColorThreshold = 100

	brightnessThrehold = 40.0

	mappingColors    = [3]*pixel{redColor, greenColor, blueColor}
	detectVideoColor = [3]string{"red", "green", "blue"}
)

// Enable Webcam Save Img if value is "on".
var enableWebcamSaveImg = testing.RegisterVarString(
	"api.enableWebcamSaveImg",
	"on",
	"WWCB enable webcam save image",
)

// CameraServiceHelper is a utility class for interacting with a camera service.
type CameraServiceHelper struct {
	service        CameraService
	exposureTimeUs map[string]int32
}

// NewCameraServiceHelper creates a new helper to wrap a camera service.
func NewCameraServiceHelper(service CameraService) *CameraServiceHelper {
	return &CameraServiceHelper{
		service:        service,
		exposureTimeUs: make(map[string]int32),
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
		if _, ok := c.exposureTimeUs[camera.GetId()]; !ok {
			c.exposureTimeUs[camera.GetId()] = 0
		}
	}
	return nil
}

// VerifyVideo verifies that the camera video contains the set frames in order.
func (c *CameraServiceHelper) VerifyVideo(ctx context.Context, outDir, cameraID string, duration int) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
	defer cancel()

	detectColorCount := 0
	for timeoutCtx.Err() == nil {
		req := &passport.GetAveragePixelRequest{DeviceId: cameraID, ExposureMicroseconds: c.exposureTimeUs[cameraID]}
		resp, err := c.service.GetAveragePixel(ctx, req)
		if err != nil {
			return errors.Wrap(err, "failed to get average pixel color from webcam")
		}

		if err := saveImageIfRequested(ctx, outDir, cameraID, "_verify_", resp.GetFrame()); err != nil {
			return errors.New("webcam with '" + cameraID + "' webcam write file error")
		}

		frameColor := detectColor(ctx, resp.GetPixel())
		testing.ContextLog(ctx, "Detect color: "+frameColor)
		testing.ContextLogf(ctx, "Expected color %s", detectVideoColor[detectColorCount])
		if frameColor == detectVideoColor[detectColorCount] {
			testing.ContextLogf(ctx, "%s color detected, incrementing colorCount", frameColor)
			detectColorCount++

			if detectColorCount == len(detectVideoColor) {
				break
			}
			// GoBigSleepLint: successfully identified a frame, wait 1 second for the next frame to appear.
			testing.Sleep(ctx, 1*time.Second)
		} else if detectColorCount > 0 && frameColor != detectVideoColor[detectColorCount-1] {
			// If we detected colors out of order then reset the count and start over.
			detectColorCount = 0
		}
	}

	if detectColorCount < len(detectVideoColor) {
		return errors.Errorf("failed to detect all colors, only found %d out of %d", detectColorCount, len(detectVideoColor))
	}

	return nil
}

// GAMLightingValue gets the average pixel "strength" between [0(dark), 255(light)].
func (c *CameraServiceHelper) GAMLightingValue(ctx context.Context, outDir, camera string) (int, error) {
	req := &passport.GetAveragePixelRequest{DeviceId: camera, ExposureMicroseconds: c.exposureTimeUs[camera]}
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
	req := &passport.GetAveragePixelRequest{DeviceId: camera, ExposureMicroseconds: c.exposureTimeUs[camera]}
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

func setInternalDisplayFullBrightness(ctx context.Context, s *testing.State) {
	err := s.DUT().Conn().CommandContext(ctx, "backlight_tool", "--set_brightness_percent=100").Run(testexec.DumpLogOnError)
	if err != nil {
		testing.ContextLog(ctx, "Failed to set internal display brightness to 100: ", err)
	}
}

// PairWebcamToDisplay matches a camera with a given display.
// It returns a map of display IDs to camera IDs.
func (c *CameraServiceHelper) PairWebcamToDisplay(ctx context.Context, s *testing.State, outDir string, displayIDs []string) (map[string]string, error) {
	// improve matching to color by setting the internal display to full brightness
	setInternalDisplayFullBrightness(ctx, s)
	resp, err := c.service.GetCameras(ctx, &passport.GetCamerasRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get available webcams")
	}

	if len(displayIDs) > len(resp.GetCameras()) {
		return nil, errors.Errorf("expect the number of online webcams is higher than the number of displays; webcams: %d, displays: %d", len(resp.GetCameras()), len(displayIDs))
	}
	var cameraIDs []string
	for _, camera := range resp.GetCameras() {
		cameraIDs = append(cameraIDs, camera.GetId())
	}
	exposureTimesToTry := []int32{750, 1000, 1500, 2250, 3000, 6000, 12000, 24000, 36000, 48000, 0}
	exposureTimeIdx := 0
	displayMappings := make(map[string]string)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Try different exposure times to see if we can get a good match.
		// testing shows that lower exposure times are more reliable and less
		// affected by glare and overexposure, so start with those and then try 0
		// which is auto exposure as a last resort.
		for _, cameraID := range cameraIDs {
			if !cameraAlreadyMatched(displayMappings, cameraID) {
				c.exposureTimeUs[cameraID] = exposureTimesToTry[exposureTimeIdx]
			}
		}
		exposureTimeIdx++
		if exposureTimeIdx == len(exposureTimesToTry) {
			exposureTimeIdx = 0
		}
		errList := c.findCameraMatch(ctx, outDir, cameraIDs, displayIDs, displayMappings)
		if len(errList) > 0 {
			return errors.Errorf("%v", errList)
		}
		return nil
	}, &testing.PollOptions{Timeout: 45 * time.Second, Interval: 1 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to match displays to cameras")
	}
	return displayMappings, nil
}

func cameraAlreadyMatched(displayMappings map[string]string, cameraID string) bool {
	for _, v := range displayMappings {
		if v == cameraID {
			return true
		}
	}
	return false
}

// findCameraMatch finds the matching camera for each display.
func (c *CameraServiceHelper) findCameraMatch(ctx context.Context, outDir string, cameraIDs, displayIDs []string, displayMappings map[string]string) []error {
	// Get current image for each camera
	camPxl := make(map[string]*passport.Pixel)
	for _, webcam := range cameraIDs {
		if cameraAlreadyMatched(displayMappings, webcam) {
			continue
		}
		testing.ContextLogf(ctx, "Trying to match camera %s with exposure of %d", webcam, c.exposureTimeUs[webcam])
		req := &passport.GetAveragePixelRequest{DeviceId: webcam, ExposureMicroseconds: c.exposureTimeUs[webcam]}
		resp, err := c.service.GetAveragePixel(ctx, req)
		if err != nil {
			return []error{errors.Wrap(err, "get average pixel from webcam")}
		}

		if err := saveImageIfRequested(ctx, outDir, webcam, "mapping", resp.GetFrame()); err != nil {
			return []error{errors.Wrapf(err, "failed to save frame for webcam: %q", webcam)}
		}

		camPxl[webcam] = resp.GetPixel()
	}

	errList := make([]error, 0)
	for dispIndex, dispID := range displayIDs {
		if _, ok := displayMappings[dispID]; ok {
			continue
		}
		expectedColor := mappingColors[dispIndex]
		testing.ContextLogf(ctx, "==== Display %d: %s ====", dispIndex, dispID)

		maxScore, cameraDev := getMaxScoreAndCamera(ctx, camPxl, dispIndex)
		testing.ContextLogf(ctx, "Max score: %d, cameraDev: %s", maxScore, cameraDev)
		if cameraDev == "" {
			err := errors.Errorf("no camera detected a display with dominant color %v for display %d, %s", expectedColor, dispIndex, dispID)
			errList = append(errList, err)
			continue
		}
		p := camPxl[cameraDev]
		brightnessVal := brightness(p)
		if brightnessVal < brightnessThrehold {
			testing.ContextLogf(ctx, "camera %s with brightness %f is too dark, not attempting to match it to a display", cameraDev, brightnessVal)
			err := errors.Errorf("camera: %s with brightness %f is too dark, not attempting to match it to a display", cameraDev, brightnessVal)
			errList = append(errList, err)
			continue
		}
		grayScore := scalarScore(p, grayColor)
		expectColorScore := scalarScore(p, expectedColor)
		// if gray score is higher than expected color score, then the display is
		// most likely off, or camera is misaligned so return an error, but only
		// check the gray score if we are below one of the color thresholds as
		// always checking would cause false negatives dut to limitations of this
		// method.
		if (int32(expectColorScore) < webcamMappingLimitScore ||
			maxScore < expectedColorThreshold) &&
			grayScore > expectColorScore {
			err := errors.Errorf("'display: %d, %s' is abnormal. Please check if the display is on. (Color scores lower than thresholds and GrayScore (%f) is higher than ColorScore (%f))", dispIndex, dispID, grayScore, expectColorScore)
			errList = append(errList, err)
			continue
		}

		// Delete this camera so we don't assign it to another display.
		delete(camPxl, cameraDev)
		testing.ContextLogf(ctx, "mapping %s to Display %d, %s with score %d and color scores color:%d, grey: %d", cameraDev, dispIndex, dispID, maxScore, int(expectColorScore), int(grayScore))
		displayMappings[dispID] = cameraDev
	}
	return errList
}

// getMaxScoreAndCamera gets the max score for the expected color and the camera device that has the max score.
//
// The score is the pixel value of the color expected for the display or -1 if
// the expected color is not the highest scoring color.
func getMaxScoreAndCamera(ctx context.Context, camPxl map[string]*passport.Pixel, dispIndex int) (int, string) {
	// Get the camera that is the closest i.e. has the highest score.
	// The score is the value of the Red, Green, or Blue channel in the color
	// (passport.Pixel), based on the dispIndex (0 for Red, 1 for Green, 2 for
	// Blue), or -1 if the score is not higher than other channel values in the
	// pixel color.
	var maxScore int32 = -1
	mappingPort := ""
	for cam, pxl := range camPxl {
		rgbScore := []int32{pxl.R, pxl.G, pxl.B}
		imgScore := rgbScore[dispIndex]
		for i := 0; i < len(rgbScore); i++ {
			// discard score if the imgScore (expected color) is not the highest
			// scoring color for the camera
			if i != dispIndex && rgbScore[i] >= rgbScore[dispIndex] {
				imgScore = -1
				break
			}
		}
		testing.ContextLogf(ctx, "Webcam: %s, Pixel: %v, Score: %d", cam, pxl, imgScore)
		if imgScore > maxScore {
			mappingPort = cam
			maxScore = imgScore
		}
	}
	return int(maxScore), mappingPort
}

func saveImageIfRequested(ctx context.Context, outDir, camera, annotation string, image []byte) error {
	if enableWebcamSaveImg.Value() != "on" {
		return nil
	}

	cameraID := filepath.Base(camera)
	imgFileName := fmt.Sprintf("%s_%s_%s_%s.jpeg", "a", annotation, cameraID, time.Now().Format("15:04:05"))
	imgFileName = path.Join(outDir, imgFileName)
	return os.WriteFile(imgFileName, image, 0644)
}

// detectColor is for detect color from pixel.
func detectColor(ctx context.Context, p *pixel) string {
	redColorDetect := &pixel{R: 240, G: 0, B: 0, A: 255}
	greenColorDetect := &pixel{R: 0, G: 240, B: 0, A: 255}
	blueColorDetect := &pixel{R: 0, G: 0, B: 240, A: 255}
	redScore := int(scalarScore(p, redColorDetect))
	greenScore := int(scalarScore(p, greenColorDetect))
	blueScore := int(scalarScore(p, blueColorDetect))

	maxScore := max(redScore, greenScore, blueScore)
	testing.ContextLog(ctx, "detectColor scores (r,g,b,max):", redScore, greenScore, blueScore, maxScore)
	if blueScore == maxScore {
		return "blue"
	} else if greenScore == maxScore {
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

// brightness gets the average pixel value across r/g/b
// to determine an estimated brightness of the pixel.
func brightness(p *pixel) float64 {
	return float64(p.R+p.G+p.B) / 3.0
}
