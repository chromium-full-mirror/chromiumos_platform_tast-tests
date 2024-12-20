// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: V4L2Compliance,
		Desc: "Runs V4L2Compliance in all the Capture Devices",
		Contacts: []string{
			"chromeos-camera-kernel@google.com",
			"chromeos-camera-eng@google.com",
			"ribalda@chromium.org",
		},
		BugComponent: "b:1481072", // ChromeOS > Platform > Technologies > Camera > Kernel
		Attr:         []string{"group:mainline", "group:camera-usb-qual", "group:cq-medium", "group:camera", "camera_kernel", "camera_functional"},
		// TODO(b/173778998) Jinlon privacy switch is not compliance: EBUSY during streamoff.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("jinlon")),
		SoftwareDeps: []string{"uvc_compliant"},
		Fixture:      fixture.CameraServiceStopped,
	})
}

func V4L2Compliance(ctx context.Context, s *testing.State) {
	badCameras := map[string]string{
		"13d3:5519": "b/258798506", // Focus out of range
		"0c45:636e": "b/281539980", // Camera in Factory Mode
		"0c45:6a16": "b/385324740", // Backlight and Contrast out of range
		"3277:0003": "b/383995807", // Focus return EIO
		"0408:302f": "b/384912098", // AutoExposure has bad flags
	}

	captureDevices, err := testutil.CaptureDevicesFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to list Capture Devices: ", err)
	}

	for _, videodev := range captureDevices {

		usbCameraVersion, err := testutil.GetUsbCameraVersion(ctx, videodev)
		if err == nil {
			vidPid := usbCameraVersion.IDVendor + ":" + usbCameraVersion.IDProduct
			bugNumber, found := badCameras[vidPid]
			if found {
				testing.ContextLog(ctx, "Device "+vidPid+" ignored due to: "+bugNumber)
				continue
			}
		}

		cmd := testexec.CommandContext(ctx, "v4l2-compliance", "-v", "-d", videodev)
		out, err := cmd.Output(testexec.DumpLogOnError)

		if err == nil {
			continue
		}

		// Log full output on error.
		result := string(out)
		s.Log(result)

		if cmd.ProcessState.ExitCode() != 1 {
			s.Fatalf("v4l2-compliance failed: %s: %v", videodev, err)
		}

		// Remove last end of line if present.
		result = strings.TrimSuffix(result, "\n")
		// Get last line.
		lastline := result[strings.LastIndex(result, "\n"):]
		s.Errorf("v4l2-compliance failed: %s: %s", videodev, lastline)
	}
}
