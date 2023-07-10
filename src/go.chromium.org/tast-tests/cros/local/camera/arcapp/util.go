// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcapp provides utilities to interact with ARC Camera Test App.
package arcapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

const (
	// CameraAppApk is the APK name of the ARC Camera Test App.
	CameraAppApk = "ArcCameraTest.apk"

	cameraAppActivity       = ".MainActivity"
	orientationTestActivity = ".OrientationTestActivity"

	cameraAppPackage = "chromeos.camera.app.arccameratest"

	intentSwitchCamera             = "chromeos.camera.app.arccameratest.ACTION_SWITCH_CAMERA"
	intentSwitchMode               = "chromeos.camera.app.arccameratest.ACTION_SWITCH_MODE"
	intentTakePhoto                = "chromeos.camera.app.arccameratest.ACTION_TAKE_PHOTO"
	intentStartRecording           = "chromeos.camera.app.arccameratest.ACTION_START_RECORDING"
	intentStopRecording            = "chromeos.camera.app.arccameratest.ACTION_STOP_RECORDING"
	intentResetCamera              = "chromeos.camera.app.arccameratest.ACTION_RESET_CAMERA"
	intentGetMetrics               = "chromeos.camera.app.arccameratest.ACTION_GET_METRICS"
	intentSetFps                   = "chromeos.camera.app.arccameratest.ACTION_SET_FPS"
	intentGetFrameDropRatio        = "chromeos.camera.app.arccameratest.ACTION_GET_FRAME_DROP_RATIO"
	intentResetMetrics             = "chromeos.camera.app.arccameratest.ACTION_RESET_METRICS"
	intentStartOrientationTest     = "chromeos.camera.app.arccameratest.ACTION_START_ORIENTATION_TEST"
	intentGetOrientationTestResult = "chromeos.camera.app.arccameratest.ACTION_GET_ORIENTATION_TEST_RESULT"
	intentGetNumOfCameras          = "chromeos.camera.app.arccameratest.ACTION_GET_NUM_OF_CAMERAS"
	keyCameraFacing                = "chromeos.camera.app.arccameratest.KEY_CAMERA_FACING"
	keyCameraMode                  = "chromeos.camera.app.arccameratest.KEY_CAMERA_MODE"
	valuePhoto                     = "Photo"
	valueVideo                     = "Video"

	// Snapshots can be really small if the room is dark, but JPEGs and MP4s are never smaller than 100 bytes.
	minExpectedFileSize = 100
)

var (
	// Sometimes it will take long after the broadcast is sent until it is received. As a result,
	// adding this flag for broadcast is helpful to raise its priority and reduce the waiting time.
	prioritizingParams = []string{"-f", "0x10000000"}
)

// ARCCameraAppMetrics represents the metrics collected by the app.
type ARCCameraAppMetrics struct {
	OpeningCamera []int64 `json:"METRIC_OPENING_CAMERA"`
	ClosingCamera []int64 `json:"METRIC_CLOSING_CAMERA"`
	TakingPhoto   []int64 `json:"METRIC_TAKING_PHOTO"`
}

type funcWithTestConn func(context.Context, *chrome.TestConn)

func broadcastIntent(ctx context.Context, a *arc.ARC, action string, params ...string) (*adb.BroadcastResult, error) {
	return a.BroadcastIntent(ctx, action, append(prioritizingParams, params...)...)
}

func broadcastIntentGetData(ctx context.Context, a *arc.ARC, action string, params ...string) (string, error) {
	return a.BroadcastIntentGetData(ctx, action, append(prioritizingParams, params...)...)
}

// LaunchARCCameraApp launches the ARC Camera Test App and returns cleanup function and error is there is any.
func LaunchARCCameraApp(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn) (funcWithTestConn, error) {
	return launchApp(ctx, a, tconn, cameraAppActivity)
}

// LaunchOrientationTestApp launches the ARC Camera Test App and lands on the orientation test activity.
func LaunchOrientationTestApp(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn) (funcWithTestConn, error) {
	return launchApp(ctx, a, tconn, orientationTestActivity)
}

func launchApp(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn, act string) (cleanupFunc funcWithTestConn, retErr error) {
	cleanupCtx := ctx
	ctx, cancelCleanup := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancelCleanup()

	activity, err := arc.NewActivity(a, cameraAppPackage, act)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new activity")
	}
	defer func(cleanupCtx context.Context, activity *arc.Activity) {
		if retErr != nil {
			activity.Close(cleanupCtx)
		}
	}(cleanupCtx, activity)

	permissions := []string{
		"android.permission.CAMERA",
		"android.permission.RECORD_AUDIO",
		"android.permission.READ_EXTERNAL_STORAGE",
		"android.permission.WRITE_EXTERNAL_STORAGE"}
	for _, permission := range permissions {
		if err := a.Command(ctx, "pm", "grant", cameraAppPackage, permission).Run(testexec.DumpLogOnError); err != nil {
			return nil, errors.Wrapf(err, "failed to grant permission %v", permission)
		}
	}

	if err = activity.StartWithDefaultOptions(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to start app")
	}
	if err := ash.WaitForVisible(ctx, tconn, cameraAppPackage); err != nil {
		return nil, errors.Wrap(err, "failed to wait for activity to be visible")
	}
	cleanupFunc = func(cleanupCtx context.Context, tconn *chrome.TestConn) {
		defer activity.Close(cleanupCtx)
		defer activity.Stop(cleanupCtx, tconn)
	}
	return cleanupFunc, nil
}

// TakePhoto asks ArcCameraTest app to take a photo via intent and ensures that
// the captured photo is saved successfully.
func TakePhoto(ctx context.Context, cr *chrome.Chrome, a *arc.ARC) error {
	if _, err := broadcastIntent(ctx, a, intentSwitchMode, "--es", keyCameraMode, valuePhoto); err != nil {
		return errors.Wrap(err, "failed to switch to photo mode")
	}

	outputFile, err := broadcastIntentGetData(ctx, a, intentTakePhoto)
	if err != nil {
		return errors.Wrap(err, "could not send intent")
	}

	// Check if photo file was generated.
	if fileSize, err := fileSizeInDCIM(ctx, cr.NormalizedUser(), outputFile); err != nil {
		return errors.Wrap(err, "could not determine size of photo file")
	} else if fileSize < minExpectedFileSize {
		return errors.Wrapf(err, "photo file is smaller than expected: got %d, want >= %d", fileSize, minExpectedFileSize)
	}
	return nil
}

// StartRecording switches to the video mode and starts the recording.
func StartRecording(ctx context.Context, cr *chrome.Chrome, a *arc.ARC) error {
	if _, err := broadcastIntent(ctx, a, intentSwitchMode, "--es", keyCameraMode, valueVideo); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}

	// Start record video
	if _, err := broadcastIntent(ctx, a, intentStartRecording); err != nil {
		return errors.Wrap(err, "could not send intent")
	}

	return nil
}

// StopRecording stops the current recording and verify the video size is reasonable.
func StopRecording(ctx context.Context, cr *chrome.Chrome, a *arc.ARC) error {
	outputFile, err := broadcastIntentGetData(ctx, a, intentStopRecording)
	if err != nil {
		return errors.Wrap(err, "could not send intent")
	}

	// Check if video file was generated.
	if fileSize, err := fileSizeInDCIM(ctx, cr.NormalizedUser(), outputFile); err != nil {
		return errors.Wrap(err, "could not determine size of video file")
	} else if fileSize < minExpectedFileSize {
		return errors.Wrapf(err, "video file is smaller than expected: got %d, want >= %d", fileSize, minExpectedFileSize)
	}
	return nil
}

// GetNumOfCameras returns the number of the detected cameras.
func GetNumOfCameras(ctx context.Context, a *arc.ARC) (int, error) {
	rawData, err := broadcastIntentGetData(ctx, a, intentGetNumOfCameras)
	if err != nil {
		return -1, errors.Wrap(err, "could not get number of cameras")
	}
	numOfCameras, err := strconv.Atoi(rawData)
	if err != nil {
		return -1, errors.Wrap(err, "failed to parse returned value when getting number of cameras")
	}
	return numOfCameras, nil
}

// SwitchCamera switches to the next camera.
func SwitchCamera(ctx context.Context, a *arc.ARC) error {
	if _, err := broadcastIntent(ctx, a, intentSwitchCamera); err != nil {
		return errors.Wrap(err, "failed to request switching camera")
	}
	return nil
}

// ResetCamera resets the camera usage in the app.
func ResetCamera(ctx context.Context, a *arc.ARC) error {
	if _, err := broadcastIntent(ctx, a, intentResetCamera); err != nil {
		return errors.Wrap(err, "could not reset the camera")
	}
	return nil
}

// GetMetrics query the metrics collected by the app.
func GetMetrics(ctx context.Context, a *arc.ARC) (ARCCameraAppMetrics, error) {
	metricsRawData, err := broadcastIntentGetData(ctx, a, intentGetMetrics)
	if err != nil {
		return ARCCameraAppMetrics{}, errors.Wrap(err, "could not get metrics")
	}

	// The metrics is in JSON format but the double quotes are escaped since
	// the string are passed through command line output. Therefore, we can
	// use unquote to unescape the string.
	metricsRawData, err = strconv.Unquote(`"` + metricsRawData + `"`)
	if err != nil {
		return ARCCameraAppMetrics{}, errors.Wrap(err, "could not unquote metrics data")
	}

	var metrics ARCCameraAppMetrics
	if err := json.Unmarshal([]byte(metricsRawData), &metrics); err != nil {
		return ARCCameraAppMetrics{}, errors.Wrapf(err, "failed to parse metrics: %v", metricsRawData)
	}
	return metrics, nil
}

// SetFPS specifies the target frame rate to be used in the app.
func SetFPS(ctx context.Context, a *arc.ARC, fps string) error {
	if _, err := broadcastIntent(ctx, a, intentSetFps, "--ei", "fps", fps); err != nil {
		return errors.Wrap(err, "could not send intent")
	}
	return nil
}

// GetFrameDropRatio gets the frame drop ratio calculated by the app.
func GetFrameDropRatio(ctx context.Context, a *arc.ARC) (float64, error) {
	rawData, err := broadcastIntentGetData(ctx, a, intentGetFrameDropRatio)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get frame drop ratio")
	}
	frameDropRatio, err := strconv.ParseFloat(rawData, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse frame drop ratio to float")
	}
	return frameDropRatio, nil
}

// ResetMetrics resets the collected metrics by the app.
func ResetMetrics(ctx context.Context, a *arc.ARC) error {
	if _, err := broadcastIntent(ctx, a, intentResetMetrics); err != nil {
		return errors.Wrap(err, "failed to reset metrics")
	}
	return nil
}

// StartOrientationTest starts to run orientation test in the app.
func StartOrientationTest(ctx context.Context, a *arc.ARC) error {
	if _, err := broadcastIntent(ctx, a, intentStartOrientationTest); err != nil {
		return errors.Wrap(err, "failed to start orientation test")
	}
	return nil
}

// OrientationTestPassed returns the orientation test result.
func OrientationTestPassed(ctx context.Context, a *arc.ARC) (bool, error) {
	rawData, err := broadcastIntentGetData(ctx, a, intentGetOrientationTestResult)
	if err != nil {
		return false, errors.Wrap(err, "failed to get orientation test result")
	}
	return strconv.ParseBool(rawData)
}

// fileSizeInDCIM searches the file inside Android DCIM folder and returns its size.
func fileSizeInDCIM(ctx context.Context, user, filename string) (int64, error) {
	androidDir, err := arc.AndroidDataDir(ctx, user)
	if err != nil {
		return -1, errors.Wrap(err, "failed to get Android data dir")
	}
	filePathInDCIM := filepath.Join(androidDir, "data/media/0/DCIM/", filename)

	info, err := os.Stat(filePathInDCIM)
	if err != nil {
		return -1, errors.Wrapf(err, "unable to access file %q", filePathInDCIM)
	}
	return info.Size(), nil
}

// PrepareSDCardAccess prepares host-side access to Android's SDCard partition, which should store the generated photo/video files.
func PrepareSDCardAccess(ctx context.Context, a *arc.ARC, cr *chrome.Chrome) (func(context.Context), error) {
	cleanupFunc, err := arc.MountSDCardPartitionOnHostWithSSHFSIfVirtioBlkDataEnabled(ctx, a, cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to make Android's SDCard partition available on host")
	}
	return cleanupFunc, nil
}
