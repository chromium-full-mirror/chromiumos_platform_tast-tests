// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelSmokeUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test for USB Camera Kernel Drivers",
		Contacts: []string{
			"chromeos-camera-kernel@google.com",
			"chromeos-camera-eng@google.com",
			"hidenorik@chromium.org",
		},
		BugComponent: "b:1481072", // ChromeOS > Platform > Technologies > Camera > Kernel
		Attr:         []string{"group:mainline", "informational", "group:camera-stability", "group:camera-kernelnext"},
		SoftwareDeps: []string{caps.BuiltinUSBCamera},
		Fixture:      fixture.CameraServiceStopped,
	})
}

func KernelSmokeUSB(ctx context.Context, s *testing.State) {
	if err := checkUSBCameraEnumerationIfSupported(ctx); err != nil {
		s.Fatal("Failed to enumerate all cameras: ", err)
	}

	// CaptureDevicesFromV4L2Test() only returns ones for USB cameras.
	captureDevices, err := testutil.CaptureDevicesFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to list capture devices: ", err)
	}

	if len(captureDevices) == 0 {
		s.Fatal("No capture device is enumerated")
	}

	s.Log("number of capture device: ", len(captureDevices))
	for _, videodev := range captureDevices {
		s.Log("checking ", videodev)
		cmd := testexec.CommandContext(ctx, "yavta", "--capture=10", videodev)
		if _, err := cmd.Output(testexec.DumpLogOnError); err != nil {
			s.Fatalf("yavta failed: %s: %v", videodev, err)
		}
	}
}

// checkUSBCameraEnumerationIfSupported can only fail if cros-config provides camera information
// and at least one of builtin cameras is not enumerated or BuiltinUsbCamerasFromV4L2Test() fails.
func checkUSBCameraEnumerationIfSupported(ctx context.Context) error {
	countStr, err := crosconfig.Get(ctx, "/camera", "count")
	if err != nil {
		return nil
	}
	numTotalCam, err := strconv.Atoi(countStr)
	if err != nil {
		return nil
	}

	numUsbCam := 0
	for i := 0; i < numTotalCam; i++ {
		device := fmt.Sprintf("/camera/devices/%v", i)
		interfaceStr, err := crosconfig.Get(ctx, device, "interface")
		if err != nil {
			return nil
		}
		if interfaceStr == "usb" {
			numUsbCam++
		}
	}

	usbCamsFound, err := testutil.BuiltinUsbCamerasFromV4L2Test(ctx)
	if err != nil {
		return err
	}
	numUsbCamFound := len(usbCamsFound)

	if numUsbCam != numUsbCamFound {
		return errors.Errorf("found %v, should be %v", numUsbCamFound, numUsbCam)
	}

	return nil
}
