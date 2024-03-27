// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

const (
	installCli = iota
	installDbus
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdInstallRemoteDBus,
		Desc: "Checks that fwupd can install using a remote repository via D-Bus",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com", // CrOS FWUPD
			"rishabhagr@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"fwupd"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      "prepareFwupd",
	})
}

// FwupdInstallRemoteDBus verifies that it can update a device in the system
// using a remote repository with D-Bus methods.
func FwupdInstallRemoteDBus(ctx context.Context, s *testing.State) {
	fwd := s.FixtValue().(*fwupd.FixtData).Fwupd

	// Find FakeCamera device ID by GUID.
	device, err := fwd.DeviceByGUID(ctx, fwupd.FakeWebcamGUID)
	if err != nil {
		s.Fatal("Failed to detect expected device using GUID: ", err)
	}
	// Ensure that fake device has the correct base version.
	if strings.Compare(device.Version, fwupd.FakeWebcamBaseVersion) != 0 {
		s.Fatalf("Unexpected device version: %s; want %s", device.Version, fwupd.FakeWebcamBaseVersion)
	}

	// Check if the target version is in the list.
	release, err := fwd.FindReleaseByVersion(ctx, device.DeviceId, fwupd.FakeWebcamUpdateVersion)
	if err != nil {
		s.Fatalf("Failed to find release %s: %v", fwupd.FakeWebcamUpdateVersion, err)
	}

	// Install the firmware file if needed and get the absolute file path.
	releaseFile, err := fwupd.DownloadFile(ctx, release.Uri, fwupd.CacheDir)
	if err != nil {
		s.Fatal("Failed to download release: ", err)
	}
	s.Log("Using the firmware file: ", releaseFile)

	if err := fwd.Install(device.DeviceId, releaseFile); err != nil {
		s.Fatal("Failed to install: ", err)
	}

	// Check the version after install.
	if device, err = fwd.DeviceByID(ctx, device.DeviceId); err != nil {
		s.Fatal("Failed to detect the device after flashing: ", err)
	}
	// Updated version for the fake device must match the expected version.
	if strings.Compare(device.Version, fwupd.FakeWebcamUpdateVersion) != 0 {
		s.Fatalf("Unexpected device version after update: %s; want %s", device.Version, fwupd.FakeWebcamUpdateVersion)
	}
}
