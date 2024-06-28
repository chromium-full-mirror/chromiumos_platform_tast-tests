// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides utilities to setup testing environment for camera
// tests.
package testutil

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/chromeproc"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	fakeCameraOptions = chrome.ExtraArgs(
		// The default fps of fake device is 20, but CCA requires fps >= 24.
		// Set the fps to 30 to avoid OverconstrainedError.
		"--use-fake-device-for-media-stream=fps=30")
	chromeWithFakeCamera = chrome.NewPrecondition("chrome_fake_camera", fakeCameraOptions)
)

// ChromeWithFakeCamera returns a precondition that Chrome is already logged in with fake camera.
func ChromeWithFakeCamera() testing.Precondition { return chromeWithFakeCamera }

// AppLauncher is used during the launch process of CCA. We could launch CCA
// by launchApp event, camera intent or any other ways.
type AppLauncher struct {
	LaunchApp    func(ctx context.Context, tconn *chrome.TestConn) error
	UseSWAWindow bool
}

// LaunchApp launches the camera app and handles the communication flow between tests and app.
func LaunchApp(ctx context.Context, cr *chrome.Chrome, tb *TestBridge, appLauncher AppLauncher) (*chrome.Conn, *AppWindow, error) {
	appWindow, err := tb.AppWindow(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to register app window")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	conn, err := func() (*chrome.Conn, error) {
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, err
		}
		if err := appLauncher.LaunchApp(ctx, tconn); err != nil {
			return nil, err
		}

		windowURL, err := appWindow.WaitUntilWindowBound(ctx)
		if err != nil {
			return nil, err
		}

		conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(windowURL))
		if err != nil {
			return nil, err
		}
		conn.StartProfiling(ctx)
		if err := appWindow.NotifyReady(ctx); err != nil {
			if closeErr := conn.CloseTarget(ctx); closeErr != nil {
				testing.ContextLog(ctx, "Failed to close app: ", closeErr)
			}
			if closeErr := conn.Close(); closeErr != nil {
				testing.ContextLog(ctx, "Failed to close app connection: ", closeErr)
			}
			return nil, err
		}
		return conn, nil
	}()
	if err != nil {
		if releaseErr := appWindow.Release(cleanupCtx); releaseErr != nil {
			testing.ContextLog(cleanupCtx, "Failed to release app window: ", releaseErr)
		}
		return nil, nil, err
	}
	return conn, appWindow, nil
}

// RefreshApp refreshes the camera app and rebuilds the communication flow between tests and app.
func RefreshApp(ctx context.Context, conn *chrome.Conn, tb *TestBridge) (*AppWindow, error) {
	appWindow, err := tb.AppWindow(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register app window")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()
	err = func() error {
		// Triggers refresh.
		if err := conn.Eval(ctx, "location.reload()", nil); err != nil {
			return errors.Wrap(err, "failed to trigger refresh")
		}

		_, err = appWindow.WaitUntilWindowBound(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to bind window")
		}

		if err := appWindow.NotifyReady(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for app window ready")
		}
		return nil
	}()
	if err != nil {
		if releaseErr := appWindow.Release(cleanupCtx); releaseErr != nil {
			testing.ContextLog(cleanupCtx, "Failed to release app window: ", releaseErr)
		}
		return nil, err
	}
	return appWindow, nil
}

// CloseApp closes the camera app and ensure the window is closed via autotest private API.
func CloseApp(ctx context.Context, cr *chrome.Chrome, appConn *chrome.Conn, useSWAWindow bool) error {
	if err := appConn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close target")
	}
	if !useSWAWindow {
		return nil
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		var isOpen bool
		if err := tconn.Call(ctx, &isOpen, `tast.promisify(chrome.autotestPrivate.isSystemWebAppOpen)`, apps.Camera.ID); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check if camera app is open via autotestPrivate.isSystemWebAppOpen API"))
		}
		if isOpen {
			return errors.New("unexpected result returned by autotestPrivate.isSystemWebAppOpen API: got true; want false")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// USBCamerasFromV4L2Test returns a list of usb camera paths.
func USBCamerasFromV4L2Test(ctx context.Context) ([]string, error) {
	cmd := testexec.CommandContext(ctx, "media_v4l2_test", "--list_usbcam")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// CaptureDevicesFromV4L2Test returns a list of usb camera paths.
func CaptureDevicesFromV4L2Test(ctx context.Context) ([]string, error) {
	cmd := testexec.CommandContext(ctx, "media_v4l2_test", "--list_capture_devices")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// BuiltinUsbCamerasFromV4L2Test returns a list of builtin usb camera paths.
func BuiltinUsbCamerasFromV4L2Test(ctx context.Context) ([]string, error) {
	cmd := testexec.CommandContext(ctx, "media_v4l2_test", "--list_builtin_usbcam")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// MIPICamerasFromCrOSCameraTool returns a list of MIPI camera information outputted from cros-camera-tool.
func MIPICamerasFromCrOSCameraTool(ctx context.Context) ([]map[string]string, error) {
	cmd := testexec.CommandContext(ctx, "cros-camera-tool", "modules", "list")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run cros-camera-tool")
	}
	var cams []map[string]string
	if err := json.Unmarshal(out, &cams); err != nil {
		return nil, errors.Wrap(err, "failed to parse cros-camera-tool output")
	}
	return cams, nil
}

// IsVividDriverLoaded returns whether vivid driver is loaded on the device.
func IsVividDriverLoaded(ctx context.Context) bool {
	cmd := testexec.CommandContext(ctx, "sh", "-c", "lsmod | grep -q vivid")
	return cmd.Run() == nil
}

// WaitForCameraServiceBinding returns when the connection between ash-chrome and cros camera service established.
func WaitForCameraServiceBinding(ctx context.Context) error {
	if upstart.JobExists(ctx, "cros-camera-algo") {
		if err := upstart.EnsureJobRunning(ctx, "cros-camera-algo"); err != nil {
			return errors.Wrap(err, "failed to start cros-camera-algo")
		}
	}
	if upstart.JobExists(ctx, "cros-camera-gpu-algo") {
		if err := upstart.EnsureJobRunning(ctx, "cros-camera-gpu-algo"); err != nil {
			return errors.Wrap(err, "failed to start cros-camera-gpu-algo")
		}
	}
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		return errors.Wrap(err, "failed to start cros-camera")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cmd := testexec.CommandContext(ctx, "cros_camera_connector_test", "--gtest_filter=ConnectorTest.GetInfo")
		if err := cmd.Run(testexec.DumpLogOnError); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to establish connection between ash-chrome and cros camera service")
	}

	return nil
}

// UsbCameraVersion contains the id:pid:bcdDevice triple from a video device.
type UsbCameraVersion struct {
	IDVendor  string
	IDProduct string
	BcdDevice string
}

// GetUsbCameraVersion returns the vid:pid:bcdDevice triple from a video device.
func GetUsbCameraVersion(ctx context.Context, device string) (UsbCameraVersion, error) {
	var fw UsbCameraVersion

	path := "/sys/class/video4linux/" + filepath.Base(device) + "/device"
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fw, err
	}
	path = filepath.Dir(string(path))
	out, err := os.ReadFile(path + "/idVendor")
	if err != nil {
		return fw, err
	}
	fw.IDVendor = strings.TrimSuffix(string(out), "\n")
	out, err = os.ReadFile(path + "/idProduct")
	if err != nil {
		return fw, err
	}
	fw.IDProduct = strings.TrimSuffix(string(out), "\n")
	out, err = os.ReadFile(path + "/bcdDevice")
	if err != nil {
		return fw, err
	}
	fw.BcdDevice = strings.TrimSuffix(string(out), "\n")
	return fw, nil
}

// Resolution is the dimensions of a camera stream.
type Resolution struct {
	Width  int
	Height int
}

// GetMaxCameraResolution returns the maximum output resolution of the cameras on the device.
func GetMaxCameraResolution(ctx context.Context) (Resolution, error) {
	result := Resolution{Width: 0, Height: 0}
	cmd := testexec.CommandContext(ctx, "cros_camera_connector_test", "--gtest_filter=ConnectorTest.GetInfo")
	out, err := cmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return result, errors.Wrap(err, "failed to run cros_camera_connector_test")
	}
	// Parse output logs in the form
	// ... DumpCameraInfo(): Format  0: MJPG 1920x1080  30fps
	// ... DumpCameraInfo(): Format  1: NV12 1920x1080  30fps
	// ... DumpCameraInfo(): Format  2: MJPG 1280x 960  30fps
	// TODO(kamesan): Consider making the test output json formatted data.
	re := regexp.MustCompile(`DumpCameraInfo\(\): Format\s+\d+:\s+\w+\s+(\d+)x\s*(\d+)`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return result, errors.New("no stream format found in camera info")
	}
	for _, match := range matches {
		width, err := strconv.Atoi(match[1])
		if err != nil {
			return result, errors.Wrap(err, "failed to parse stream width")
		}
		height, err := strconv.Atoi(match[2])
		if err != nil {
			return result, errors.Wrap(err, "failed to parse stream height")
		}
		if width*height > result.Width*result.Height {
			result = Resolution{Width: width, Height: height}
		}
	}
	return result, nil
}

// GetVideoCaptureServiceProcess returns video capture service process.
func GetVideoCaptureServiceProcess(ctx context.Context) (*process.Process, error) {
	const videoCaptureUtilProcName = "video_capture.mojom.VideoCaptureService"
	procs, err := chromeproc.GetUtilityProcesses()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get utility processes")
	}

	re := regexp.MustCompile(` --?utility-sub-type=([\w\.]+)(?: |$)`)

	for _, proc := range procs {
		cmdline, err := proc.Cmdline()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get cmdline")
		}

		matches := re.FindStringSubmatch(cmdline)
		if len(matches) < 2 {
			continue
		}

		procName := matches[1]
		if procName == videoCaptureUtilProcName {
			return proc, nil
		}
	}

	return nil, errors.New("failed to find video capture service process")
}

// KillVideoCaptureServiceProcess kills video capture service process. Ash will launch a
// new video capture service process after the old process is killed or crashed.
func KillVideoCaptureServiceProcess(ctx context.Context) error {
	oldProc, err := GetVideoCaptureServiceProcess(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find video capture service before killing")
	}
	oldPid := oldProc.Pid

	if err := oldProc.Kill(); err != nil {
		return errors.Wrap(err, "failed to execute kill command")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		newProc, err := GetVideoCaptureServiceProcess(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to find new video capture service after killing")
		}
		newPid := newProc.Pid
		if oldPid == newPid {
			return errors.New("failed to kill old video capture service")
		}
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 3 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to launch a new video capture service")
	}
	return nil
}

// CheckBuiltinCameraEnumeration checks whether all the builtin cameras are enumerated.
func CheckBuiltinCameraEnumeration(ctx context.Context) error {
	countStr, err := crosconfig.Get(ctx, "/camera", "count")
	if err != nil {
		return err
	}
	numTotalCam, err := strconv.Atoi(countStr)
	if err != nil {
		return err
	}

	// Detect builtin USB cameras.
	usbCams, err := BuiltinUsbCamerasFromV4L2Test(ctx)
	if err != nil {
		return err
	}
	numUsbCam := len(usbCams)

	// Detect MIPI cameras.
	mipiCams, err := MIPICamerasFromCrOSCameraTool(ctx)
	if err != nil {
		return err
	}
	numMipiCam := len(mipiCams)

	if numUsbCam+numMipiCam != numTotalCam {
		return errors.Errorf("failed to enumerate all built-in cameras (found %v usb, %v mipi)", numUsbCam, numMipiCam)
	}

	return nil
}
