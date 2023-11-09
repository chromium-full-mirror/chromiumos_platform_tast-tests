// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/autocaps"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Capability,
		Desc:         "Compare capabilities defined in autocaps package with ones detected by platform camera tools",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamesan@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-stability", "group:camera-kernelnext"},
	})
}

// Capability compares the static capabilities versus those detected in the DUT.
func Capability(ctx context.Context, s *testing.State) {
	// Get capabilities defined in autocaps package.
	staticCaps, err := autocaps.Read(autocaps.DefaultCapabilityDir, nil)
	if err != nil {
		s.Fatal("Failed to read statically-set capabilities: ", err)
	}

	// Detect builtin USB cameras.
	usbCams, err := testutil.BuiltinUsbCamerasFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to get Built-in USB cameras: ", err)
	}

	// Detect MIPI cameras.
	mipiCams, err := testutil.MIPICamerasFromCrOSCameraTool(ctx)
	if err != nil {
		s.Fatal("Failed to get MIPI cameras: ", err)
	}

	builtInUsbCams := len(usbCams)

	// Exclude detected usb camera with detachable flag true, this should
	// not be treated as built_in camera.
	for i := 0; ; i++ {
		devicePath := fmt.Sprintf("/camera/devices/%v", i)
		cameraType, err := crosconfig.Get(ctx, devicePath, "interface")
		if crosconfig.IsNotFound(err) {
			break
		}
		if err != nil {
			s.Fatal("Failed to execute cros cros config: ", err)
		}

		isDetachable, err := crosconfig.Get(ctx, devicePath, "detachable")
		if crosconfig.IsNotFound(err) {
			continue
		}
		if err != nil {
			s.Fatal("Failed to execute cros cros config: ", err)
		}

		if cameraType == "usb" && string(isDetachable) == "true" {
			builtInUsbCams--
		}
	}

	hasUSB := builtInUsbCams > 0
	hasMIPI := len(mipiCams) > 0

	hasVivid := testutil.IsVividDriverLoaded(ctx)

	capsToVerify := map[string]bool{
		"builtin_usb_camera":      hasUSB,
		"builtin_mipi_camera":     hasMIPI,
		"vivid_camera":            hasVivid,
		"builtin_camera":          hasUSB || hasMIPI,
		"builtin_or_vivid_camera": hasUSB || hasMIPI || hasVivid,
	}
	for c, detected := range capsToVerify {
		if staticCaps[c] == autocaps.Yes && !detected {
			s.Errorf("%q statically set but not detected", c)
		} else if staticCaps[c] != autocaps.Yes && detected {
			s.Errorf("%q detected but not statically set", c)
		}
	}
}
