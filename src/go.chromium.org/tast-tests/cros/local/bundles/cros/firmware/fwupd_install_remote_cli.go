// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdInstallRemoteCLI,
		Desc: "Checks that fwupd can install using a remote repository with fwupdmgr CLI",
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

// FwupdInstallRemoteCLI runs the fwupdtool utility in CLI (command line interface) and verifies that it
// can update a device in the system using a remote repository.
func FwupdInstallRemoteCLI(ctx context.Context, s *testing.State) {
	fwd := s.FixtValue().(*fwupd.FixtData).Fwupd

	// Find FakeCamera device ID by GUID.
	device, err := fwd.DeviceByGUID(ctx, fwupd.FakeWebcamGUID)
	// Ensure that fake device have correct base version.
	if strings.Compare(device.Version, fwupd.FakeWebcamBaseVersion) != 0 {
		s.Error("Unexpected device version: ", device.Version)
	}

	cmd := testexec.CommandContext(ctx, "/usr/bin/fwupdmgr", "install", "--allow-reinstall", "-v", fwupd.ReleaseURI)
	cmd.Env = append(os.Environ(), "CACHE_DIRECTORY=/var/cache/fwupd")
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("%q failed: %v", cmd.Args, err)
	}
	if err := os.WriteFile(filepath.Join(s.OutDir(), "fwupdmgr.txt"), output, 0644); err != nil {
		s.Error("Failed to write output from update: ", err)
	}

	// Check the version after install.
	if device, err = fwd.DeviceByID(ctx, device.DeviceId); err != nil {
		s.Fatal("Unable to detect the device after flashing: ", err)
	}
	// Updated version for the fake device must match the expected version.
	if strings.Compare(device.Version, fwupd.FakeWebcamUpdateVersion) != 0 {
		s.Fatal("Unexpected device version after update: expected ", fwupd.FakeWebcamUpdateVersion, ", got ", device.Version)
	}
}
