// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdDetectRTD2142,
		Desc: "Checks that fwupd can detect realtek-mst devices",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com", // CrOS FWUPD
			"pmarheine@chromium.org",    // Test Author
		},
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"fwupd"},
		HardwareDeps: hwdep.D(
			hwdep.DisplayPortConverter("RTD2142"),
		),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// FwupdDetectRTD2142 gets devices from fwupd via dbus and verifies that a RTD2142 is recognized.
func FwupdDetectRTD2142(ctx context.Context, s *testing.State) {
	const (
		expectedDeviceName       = "RTD2142"
		expectedDeviceInstanceID = `I2C\NAME_10EC2142:00&FAMILY_Google_Hatch`
		expectedDeviceGUID       = "d02db69e-478f-5598-86f2-60bb5e026461"
		expectedPlugin           = "realtek_mst"
	)

	fwd, err := fwupd.Init()
	if err != nil {
		s.Fatal("Failed to connect to fwupd: ", err)
	}

	device, err := fwd.DeviceByGUID(ctx, expectedDeviceGUID)
	if err != nil {
		testing.ContextLog(ctx, "On some devices (particularly if in a"+
			" pre-MP build phase), the MST firmware may be too"+
			" old to support detection and needs to be replaced:"+
			" this failure may be a problem with the device the test"+
			" ran on, not a bug in this test or fwupd. See"+
			" https://issuetracker.google.com/issues/173742142#comment30"+
			" for details.")
		s.Fatal("Failed to detect expected device: ", err)
	}

	foundInstanceID := false
	for _, instanceID := range device.InstanceIds {
		if instanceID == expectedDeviceInstanceID {
			foundInstanceID = true
		}
	}
	if !foundInstanceID {
		s.Errorf("Failed to find expected instance ID %q among %q", expectedDeviceInstanceID, device.InstanceIds)
	}

	if device.Name != expectedDeviceName {
		s.Errorf("Failed to verify device name: expected %q, got %q", expectedDeviceName, device.Name)
	}

	if device.Plugin != expectedPlugin {
		s.Errorf("Failed to verify plugin: expected %q, got %q", expectedPlugin, device.Plugin)
	}
}
