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
		Func: FwupdDetectPS175,
		Desc: "Checks that fwupd can detect device",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com", // CrOS FWUPD
			"pmarheine@chromium.org",    // Test Author
		},
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"fwupd"},
		HardwareDeps: hwdep.D(
			hwdep.DisplayPortConverter("PS175"),
		),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// FwupdDetectPS175 gets devices from the fwupd dbus service and verifies that
// a PS175 exists with expected property values.
func FwupdDetectPS175(ctx context.Context, s *testing.State) {
	const (
		expectedDeviceName       = "PS175"
		expectedDeviceInstanceID = `I2C\NAME_1AF80175:00`
		expectedDeviceGUID       = "c146ccc9-58b6-517c-97f6-9c55a0bd39d3"
		expectedPlugin           = "parade_lspcon"
	)

	device, err := fwupd.GetDeviceByGUID(ctx, expectedDeviceGUID)
	if err != nil {
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
