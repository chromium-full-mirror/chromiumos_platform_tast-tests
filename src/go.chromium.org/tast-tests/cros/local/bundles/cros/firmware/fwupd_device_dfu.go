// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

const (
	paramUpdate = iota
	paramDowngrade
	paramInstallFile
	paramInstallVer
)

type fwupdOp struct {
	action int
	opts   []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdDeviceDfu,
		Desc: "Checks the update and downgrade devices supported by fwupd",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com",  // Owning team mailing list
			"denis.pynkin@collabora.com", // Optional test contact
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"fwupd"},
		Vars: []string{
			"fwupd.deviceId",
			"fwupd.deviceGuid",
			"fwupd.fwFile",
			"fwupd.fwVersion",
		},
		Params: []testing.Param{{
			Name: "downgrade",
			Val: fwupdOp{
				action: paramDowngrade,
				// Using install from URI instead of downgrade to avoid interactive mode.
				opts: []string{"install", "--json", "--assume-yes", "--allow-older"},
			},
			Fixture: "ensureRemotes",
		}, {
			Name: "install_file",
			Val: fwupdOp{
				action: paramInstallFile,
				opts:   []string{"install", "--json", "--assume-yes", "--allow-older", "--allow-reinstall"},
			},
			// Remotes are not needed for this test.
		}, {
			Name: "install_version",
			Val: fwupdOp{
				action: paramInstallVer,
				opts:   []string{"install", "--json", "--assume-yes", "--allow-older", "--allow-reinstall"},
			},
			Fixture: "ensureRemotes",
		}, {
			Name: "update",
			Val: fwupdOp{
				action: paramUpdate,
				opts:   []string{"update", "--json", "--assume-yes"},
			},
			Fixture: "ensureRemotes",
		}},
	})
}

func FwupdDeviceDfu(ctx context.Context, s *testing.State) {
	fwupdVersion, err := fwupd.Version(ctx)
	if err != nil {
		s.Fatal("Unable to get FWUPD version: ", err)
	}
	s.Log("FWUPD version detected: ", fwupdVersion)

	var device *fwupd.Device
	var target string
	// Device ID or GUID are required,
	// if both passed -- the device ID is preferable.
	if expectedID, ok := s.Var("fwupd.deviceId"); ok {
		s.Log("Expecting device ID: ", expectedID)
		device, err = fwupd.DeviceByID(ctx, expectedID)
		if err != nil {
			s.Fatal("Failed to detect expected device: ", err)
		}
		target = device.DeviceId
	} else if expectedGUID, ok := s.Var("fwupd.deviceGuid"); ok {
		device, err = fwupd.DeviceByGUID(ctx, expectedGUID)
		if err != nil {
			s.Fatal("Failed to detect expected device: ", err)
		}
		target = expectedGUID
	} else {
		s.Fatal("Requires variable 'fwupd.deviceId' or 'fwupd.deviceGuid' supplied via -var or -varsfile")
	}

	if device.Problems != 0 {
		s.Fatal("Unable to use '" + device.Name + "' due detected problems: " + device.UpdateError)
	}

	testParams := s.Param().(fwupdOp)
	opts := testParams.opts

	switch testParams.action {
	case paramUpdate:
		opts = append(opts, target)
	case paramDowngrade:
		var fwVer string
		if fwVer, err = fwupd.DeviceDowngradeVersion(ctx, device.DeviceId); err != nil {
			s.Fatal("Error fetching downgrades for device '" + device.Name + "': " + device.UpdateError)
		}
		s.Log("Found version for downgrade: ", fwVer)
		opts = append(opts, target)
		opts = append(opts, fwVer)
	case paramInstallFile:
		fwFile := s.RequiredVar("fwupd.fwFile")
		opts = append(opts, fwFile)
		opts = append(opts, target)
		os.Setenv("CACHE_DIRECTORY", "/var/cache/fwupd")
	case paramInstallVer:
		fwVer := s.RequiredVar("fwupd.fwVersion")
		opts = append(opts, target)
		opts = append(opts, fwVer)
	default:
		s.Fatal("Unknown parameter")
	}

	versionPre := device.Version

	cmd := testexec.CommandContext(ctx, "/usr/bin/fwupdmgr", opts[:]...)
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("%s failed: %v", shutil.EscapeSlice(cmd.Args), err)
	}
	if err := os.WriteFile(filepath.Join(s.OutDir(), "fwupdmgr.txt"), output, 0644); err != nil {
		s.Error("Failed dumping fwupdmgr output: ", err)
	}

	if !strings.Contains(string(output), "Successfully installed firmware") {
		s.Fatal("Error updating firmware for device ", device.Name)
	}

	// Check the version after flashing.
	if device, err = fwupd.DeviceByID(ctx, device.DeviceId); err != nil {
		s.Fatal("Unable to detect the device after flashing: ", err)
	}

	if strings.Compare(versionPre, device.Version) == 0 {
		s.Fatal("Error updating firmware for device ", device.Name, ": the FW release version hasn't changed (", device.Version, ")")
	}

	// Send signed report only for update.
	if testParams.action == paramUpdate {
		cmd := testexec.CommandContext(ctx, "/usr/bin/fwupdmgr", "report-history", "--assume-yes", "--sign", "--json")
		if _, err := cmd.Output(testexec.DumpLogOnError); err != nil {
			// Do not fail the test.
			s.Log("Unable to send the report: ", err)
		} else {
			s.Log("Report sent successfully")
		}
	}
}
