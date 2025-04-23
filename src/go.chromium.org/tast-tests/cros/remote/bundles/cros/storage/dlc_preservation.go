// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dlcPath     = "/mnt/stateful_partition/unencrypted/dlc-factory-images/foobar/package"
	dlcFilePath = dlcPath + "/dlc.img"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DLCPreservation,
		Desc: "Validation test for DLC preservation",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func createFile(ctx context.Context, dut *dut.DUT) error {

	// Creating directory.
	cmd := fmt.Sprintf("mkdir -p %v", dlcPath)
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create dir")
	}

	// Creating dlc image file.
	cmd = fmt.Sprintf("echo foobar > %s", dlcFilePath)
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create dlc file")
	}
	return nil
}

func DLCPreservation(ctx context.Context, s *testing.State) {

	dut := s.DUT()

	// This is to setting up boot from usb.
	err := util.BootFromUSB(ctx, dut)
	if err != nil {
		s.Fatal("Unable to set boot from usb: ", err)
	}

	// Creating dlc.img at path "/mnt/stateful_partition/unencrypted/dlc-factory-images/foobar/package/".
	err = createFile(ctx, dut)
	if err != nil {
		s.Fatal("Failed to create dlc file: ", err)
	}
	// Function to install chromeOS using usb drive.
	err = util.InstallChromeOS(ctx, dut)
	if err != nil {
		s.Fatal("Error while installing the chromeOS: ", err)
	}

	// Checking if dlc.img is exists at path /mnt/stateful_partition/unencrypted/.
	exists, err := fileutils.HostFileExists(ctx, dut.Conn(), dlcFilePath)

	if err != nil {
		s.Fatal("File dlc.img could not be checked: ", err)
	} else if !exists {
		s.Fatal("File dlc.img does not exist")
	}

}
