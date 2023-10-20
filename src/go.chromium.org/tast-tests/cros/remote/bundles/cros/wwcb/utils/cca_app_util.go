// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/media/imgcmp"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

var (
	// CameraWindowFinder is the finder of camera ap window.
	CameraWindowFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Camera"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// startRecordFinder is the finder used to start recording on the camera app.
	startRecordFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "shutter"}},
			{Value: &ui.NodeWith_Name{Name: "Start recording"}},
			{Value: &ui.NodeWith_Focusable{}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}

	// stopRecordFinder is the finder used to stop recording on the camera app.
	stopRecordFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "shutter"}},
			{Value: &ui.NodeWith_Name{Name: "Stop recording"}},
			{Value: &ui.NodeWith_Focusable{}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}

	// takePhotoFinder is the finder used to take photo on the camera app.
	takePhotoFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "shutter"}},
			{Value: &ui.NodeWith_Name{Name: "Take photo"}},
			{Value: &ui.NodeWith_Focusable{}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}

	// previewContentFinder is the finder used to preview camera.
	previewContentFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "preview-content"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_CANVAS}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}

	// videoModeFinder is the finder used to switch to record video mode on the camera app.
	videoModeFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Switch to record video"}},
			{Value: &ui.NodeWith_Focusable{}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}

	// photoModeFinder is the finder used to switch to take photo mode on the camera app.
	photoModeFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Switch to take photo"}},
			{Value: &ui.NodeWith_Focusable{}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}

	// switchDeviceFinder is the finder used to switch camera device on the camera app.
	switchDeviceFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Switch to next camera"}},
			{Value: &ui.NodeWith_Focusable{}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: CameraWindowFinder}},
		},
	}
)

// CameraPath is an absolute path on DUT.
const CameraPath = "/home/chronos/user/MyFiles/Camera"

const (
	// VideoMode is the mode used to record video.
	VideoMode = "video"
	// PhotoMode is the mode used to take photo.
	PhotoMode = "photo"
	// CCAPlugged is cca plugged info
	CCAPlugged = ".*is plugged in"
	// CCAUnplugged is cca unplugged info
	CCAUnplugged = ".*is unplugged"
)

var (
	// PhotoPattern is the filename format of photos taken by CCA.
	PhotoPattern = regexp.MustCompile(`^IMG_\d{8}_\d{6}[^.]*\.jpg$`)
	// VideoPattern is the filename format of videos recorded by CCA.
	VideoPattern = regexp.MustCompile(`^VID_\d{8}_\d{6}[^.]*\.mp4$`)
)

// WaitForFinderLocationStable waits for the finder to be stable, since the app take time to present.
func WaitForFinderLocationStable(ctx context.Context, uiautoSvc ui.AutomationServiceClient, finder *ui.Finder) error {
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
		return errors.Wrap(err, "failed to wait until the finder exists")
	}

	var finderLocation string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: finder})
		if err != nil {
			return errors.Wrap(err, "failed to get the finder info")
		}

		if resp.NodeInfo.Location.String() != finderLocation {
			finderLocation = resp.NodeInfo.Location.String()
			return errors.New("Unable to find the stable finder location")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}

// TakeSinglePhoto returns a file info using the Camera app to capture a photo.
func TakeSinglePhoto(ctx context.Context, uiautoSvc ui.AutomationServiceClient, fs *dutfs.Client, dir string) (os.FileInfo, error) {
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: takePhotoFinder}); err != nil {
		return nil, errors.Wrap(err, "failed to wait for shutter button from context menu")
	}

	photoStartTime := time.Now()
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: takePhotoFinder}); err != nil {
		return nil, errors.Wrap(err, "failed to click shutter button from context menu")
	}

	photoInfo, err := waitForFileSaved(ctx, fs, dir, PhotoPattern, photoStartTime, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for photo file saved")
	}
	return photoInfo, nil
}

// RecordVideo returns a file info using the Camera app to record video.
func RecordVideo(ctx context.Context, uiautoSvc ui.AutomationServiceClient, fs *dutfs.Client, d time.Duration, dir string) (os.FileInfo, error) {
	videoStartTime := time.Now()
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: startRecordFinder}); err != nil {
		return nil, errors.Wrap(err, "failed to wait for start recording button from context menu")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: startRecordFinder}); err != nil {
		return nil, errors.Wrap(err, "failed to click the start recording button from context menu")
	}

	// GoBigSleepLint: Wait for recording video.
	testing.Sleep(ctx, d)

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: stopRecordFinder}); err != nil {
		return nil, errors.Wrap(err, "failed to click the stop recording button from context menu")
	}

	videoFileInfo, err := waitForFileSaved(ctx, fs, dir, VideoPattern, videoStartTime, 15*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for the video file saved")
	}

	return videoFileInfo, nil
}

// SwitchCCAMode switches Camera app mode by UI automation.
func SwitchCCAMode(ctx context.Context, uiautoSvc ui.AutomationServiceClient, mode string) error {
	var modeFinder *ui.Finder
	var shutterFinder *ui.Finder
	if mode == VideoMode {
		modeFinder = videoModeFinder
		shutterFinder = startRecordFinder
	} else {
		modeFinder = photoModeFinder
		shutterFinder = takePhotoFinder
	}

	resp, _ := uiautoSvc.IsNodeFound(ctx, &ui.IsNodeFoundRequest{Finder: shutterFinder})
	if resp.Found {
		return nil
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: modeFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for mode from context menu")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: modeFinder}); err != nil {
		return errors.Wrap(err, "failed to click the mode from context menu")
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: shutterFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for shutter from context menu")
	}

	if err := WaitForFinderLocationStable(ctx, uiautoSvc, previewContentFinder); err != nil {
		return errors.Wrap(err, "failed to wait for preview content to be stable from context menu")
	}
	return nil
}

// SwitchCCACamera switches to the next camera on the camera app.
func SwitchCCACamera(ctx context.Context, dut *dut.DUT, uiautoSvc ui.AutomationServiceClient) error {
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: switchDeviceFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for switch button from context menu")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: switchDeviceFinder}); err != nil {
		return errors.Wrap(err, "failed to to click switch button from context menu")
	}
	return nil
}

// ConnectUSBDevice returns the USB device info using command "lsusb" to find the difference before and after connect the fixture.
func ConnectUSBDevice(ctx context.Context, dut *dut.DUT, usbDeviceID string) (string, error) {
	before, err := GetUSBDevice(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "failed to get USB devices")
	}

	if err := ControlFixture(ctx, usbDeviceID, "on"); err != nil {
		return "", errors.Wrapf(err, "failed to control fixture %s: ", usbDeviceID)
	}

	var usbDeviceInfo string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, err := GetUSBDevice(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "failed to get USB devices after connect the fixture")
		}
		diff := FindDifference(after, before)

		if len(diff) != 1 {
			return errors.Errorf("Expect DUT would increase one USB device, but got %v", diff)
		}

		usbDeviceInfo = diff[0]

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return "", errors.Wrap(err, "failed to find the USB device info after fixture is connected")
	}

	return usbDeviceInfo, nil
}

// waitForFileSaved waits for the presence of the captured file with file name matching the specified
// pattern, size larger than zero, and modified time after the specified timestamp.
func waitForFileSaved(ctx context.Context, fs *dutfs.Client, dir string, pat *regexp.Regexp, ts time.Time, timeout time.Duration) (os.FileInfo, error) {
	var result os.FileInfo
	seen := make(map[string]struct{})
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		files, err := fs.ReadDir(ctx, dir)
		if err != nil {
			return errors.Wrap(err, "failed to read the camera directory")
		}
		for _, file := range files {
			if file.Size() == 0 || file.ModTime().Before(ts) {
				continue
			}
			if _, ok := seen[file.Name()]; ok {
				continue
			}
			seen[file.Name()] = struct{}{}
			testing.ContextLog(ctx, "New file found: ", file.Name())
			if pat.MatchString(file.Name()) {
				testing.ContextLog(ctx, "Found a match: ", file.Name())
				result = file
				return nil
			}
		}
		return errors.New("no matching output file found")
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return nil, errors.Wrapf(err, "no matching output file found after %v", timeout)
	}
	return result, nil
}

// FindInfoOnCameraApp check the info of plug/unplug
func FindInfoOnCameraApp(ctx context.Context, uiautoSvc ui.AutomationServiceClient, infoRegex string) error {
	infoFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameRegex{NameRegex: infoRegex}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: infoFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for the info from context menu")
	}
	return nil
}

// ConnectExternalCamera connects an external camera through a fixture and returns device information.
func ConnectExternalCamera(ctx context.Context, dut *dut.DUT, extCameraID string) (string, error) {
	before, err := USBCamerasFromV4L2Test(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "failed to get v4l2 devices before connecting external camera")
	}

	if err := ControlFixture(ctx, extCameraID, "on"); err != nil {
		return "", errors.Wrap(err, "failed to control fixture to connect external camera")
	}

	var extCamera string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, err := USBCamerasFromV4L2Test(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "failed to get v4l2 devices after connecting external camera")
		}

		diff := FindDifference(after, before)
		if len(diff) == 1 {
			extCamera = diff[0]
			return nil
		}
		return errors.New("Unable to find the external camera")
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		return "", err
	}
	return extCamera, nil
}

// SwitchCCADevice clicks the "switch device" button on the CCA app.
func SwitchCCADevice(ctx context.Context, dut *dut.DUT, uiautoSvc ui.AutomationServiceClient, expectedDevice string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		currentDevice, err := CCAUseDevice(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve CCA app is using which camera")
		}

		if expectedDevice == currentDevice {
			return nil
		}

		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: switchDeviceFinder}); err != nil {
			return errors.Wrap(err, "failed to wait for switch button")
		}

		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: switchDeviceFinder}); err != nil {
			return errors.Wrap(err, "failed to to click switch button")
		}

		return errors.Errorf("CCA app is not using the expected camera; current: %s, expected: %v", currentDevice, expectedDevice)
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 3 * time.Second})
}

// CCAUseDevice returns the device currently being used by the CCA application.
func CCAUseDevice(ctx context.Context, dut *dut.DUT) (string, error) {
	devices, err := USBCamerasFromV4L2Test(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "failed to get v4l2 devices")
	}

	out, err := dut.Conn().CommandContext(ctx, "lsof").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "execute lsof command")
	}
	output := string(out)

	for _, device := range devices {
		if strings.Contains(output, device) && strings.Contains(output, "arc-camera") {
			return device, nil
		}
	}
	return "", errors.New("Unable to find CCA app is using which v4l2 devices")
}

// CropCCAPreview returns the preview image of the CCA application.
func CropCCAPreview(ctx context.Context, uiautoSvc ui.AutomationServiceClient, savedFolder string) (image.Image, error) {
	maxSizeOption := grpc.MaxCallRecvMsgSize(32 * 10e6)
	resp, err := uiautoSvc.CaptureScreenshot(ctx, &ui.CaptureScreenshotRequest{}, maxSizeOption)
	if err != nil {
		return nil, errors.Wrap(err, "failed to take screenshot")
	}

	img, err := png.Decode(bytes.NewReader(resp.PngBase64))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode PNG file")
	}

	builtinDisplayInfo, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: BuiltinDisplayFinder})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get built-in display info")
	}

	previewContentInfo, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: previewContentFinder})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get CCA preview content info")
	}

	zoomW := img.Bounds().Max.X / int(builtinDisplayInfo.NodeInfo.Location.Width)
	zoomH := img.Bounds().Max.Y / int(builtinDisplayInfo.NodeInfo.Location.Height)

	subX0 := int(previewContentInfo.NodeInfo.Location.Left) * zoomW
	subY0 := int(previewContentInfo.NodeInfo.Location.Top) * zoomH
	subX1 := int(previewContentInfo.NodeInfo.Location.Left+previewContentInfo.NodeInfo.Location.Width) * zoomW
	subY1 := int(previewContentInfo.NodeInfo.Location.Top+previewContentInfo.NodeInfo.Location.Height) * zoomH

	subImage := img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(subX0, subY0, subX1, subY1))

	builtinDisplayFilepath := filepath.Join(savedFolder, "builtin_display.png")
	if err := imgcmp.DumpImageToPNG(ctx, &img, builtinDisplayFilepath); err != nil {
		return nil, errors.Wrap(err, "failed to save built-in display")
	}

	ccaPreviewFilepath := filepath.Join(savedFolder, "cca_preview.png")
	if err := imgcmp.DumpImageToPNG(ctx, &subImage, ccaPreviewFilepath); err != nil {
		return nil, errors.Wrap(err, "failed to save CCA preview content image")
	}

	return subImage, nil
}
