// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdGetDevices,
		Desc: "Checks that fwupd can detect device",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com", // CrOS FWUPD
			"rishabhagr@chromium.org",
		},
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"fwupd"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// FwupdGetDevices makes the dbus call `GetDevices` and searches through them to
// find a specific fake device.
func FwupdGetDevices(ctx context.Context, s *testing.State) {
	fwupdVersion, err := fwupd.Version(ctx)
	if err != nil {
		s.Fatal("Unable to get FWUPD version: ", err)
	}
	s.Log("FWUPD version detected: ", fwupdVersion)

	// Get device using DeviceID
	var device *fwupd.Device
	device, err = fwupd.DeviceByID(ctx, fwupd.FakeWebcamDeviceID)
	if err != nil {
		s.Fatal("Failed to detect expected device using DeviceID: ", err)
	}
	if device.Name != fwupd.FakeWebcamName {
		s.Fatalf("Detected device name: %s is different from expected device name: %s", device.Name, fwupd.FakeWebcamName)
	}
	if device.Version != fwupd.FakeWebcamVersion {
		s.Fatalf("Detected device version: %s is different from expected device version: %s", device.Version, fwupd.FakeWebcamVersion)
	}
}
