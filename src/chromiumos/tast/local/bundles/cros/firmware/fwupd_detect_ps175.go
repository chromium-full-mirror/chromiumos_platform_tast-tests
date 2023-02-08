// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

const (
	dbusName      = "org.freedesktop.fwupd"
	dbusPath      = "/"
	dbusInterface = "org.freedesktop.fwupd"

	expectedDeviceName       = "PS175"
	expectedDeviceInstanceID = `I2C\NAME_1AF80175:00`
	expectedDeviceGUID       = "c146ccc9-58b6-517c-97f6-9c55a0bd39d3"
	expectedPlugin           = "parade_lspcon"
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
	})
}

// validatePS175Properties verifies that the properties of a fwupd device
// match the expected values for a PS175.
func validatePS175Properties(device map[string]dbus.Variant, s *testing.State) {
	// Check instance IDs for the one we expect.
	var instanceIDs []string
	if err := dbus.Store([]interface{}{device["InstanceIds"]}, &instanceIDs); err != nil {
		s.Fatal("Failed to store list of instance IDs: ", err)
	}
	foundInstanceID := false
	for _, instanceID := range instanceIDs {
		if instanceID == expectedDeviceInstanceID {
			foundInstanceID = true
		}
	}
	if !foundInstanceID {
		s.Errorf("Did not find expected instance ID %q among %q", expectedDeviceInstanceID, instanceIDs)
	}

	if device["Name"].Value() != expectedDeviceName {
		s.Errorf("Expected device name %q, but was %q", expectedDeviceName, device["Name"].Value())
	}

	if device["Plugin"].Value() != expectedPlugin {
		s.Errorf("Expected plugin %q, but was %q", expectedPlugin, device["Plugin"].Value())
	}
}

// FwupdDetectPS175 gets devices from the fwupd dbus service and verifies that
// a PS175 exists with expected property values.
func FwupdDetectPS175(ctx context.Context, s *testing.State) {

	conn, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("Failed to connect to system bus: ", err)
		return
	}
	defer conn.Close()

	var devices []map[string]dbus.Variant
	fwupd := conn.Object(dbusName, dbusPath)
	if err = fwupd.Call(dbusInterface+".GetDevices", 0).Store(&devices); err != nil {
		s.Fatal("Failed to call GetDevices: ", err)
		return
	}

	// Scan all devices to locate one with the expected GUID
	var foundDevice map[string]dbus.Variant
scanDevices:
	for _, device := range devices {
		s.Logf("Inspecting device: %q", device)

		var guids []string
		if err := dbus.Store([]interface{}{device["Guid"]}, &guids); err != nil {
			s.Fatal("Failed to store list of GUIDs: ", err)
		}
		for _, guid := range guids {
			if guid == expectedDeviceGUID {
				foundDevice = device
				break scanDevices
			}
		}
	}

	if foundDevice == nil {
		s.Fatalf("No device found with expected GUID (%q)", expectedDeviceGUID)
	} else {
		validatePS175Properties(foundDevice, s)
	}
}
