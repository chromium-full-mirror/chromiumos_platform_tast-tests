// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TransitionReFSI,
		Desc: "Test for Transition of reFSI boards to new layout on forced auto-update",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:storage_destructive"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func checkLVMOutput(ctx context.Context, dut *dut.DUT, s *testing.State) (bool, error) {

	// This is to check the lvs output.
	out, err := util.RunCmdWithStringOutput(ctx, dut, "sh", "-c", "lvs")
	if err != nil {
		return false, errors.Wrap(err, "failed to lvs output")
	}
	return out == "", nil
}

func TransitionReFSI(ctx context.Context, s *testing.State) {

	dut := s.DUT()

	// Setting up boot from usb.
	err := util.BootFromUSB(ctx, dut)
	if err != nil {
		s.Fatal("Unable to set boot from usb: ", err)
	}

	// This function installs chromeOS using usb drive.
	err = util.InstallChromeOS(ctx, dut, false)
	if err != nil {
		s.Fatal("Error while installing the chromeOS: ", err)
	}

	// Function to check LVM output.
	isEmpty, err := checkLVMOutput(ctx, dut, s)
	if err != nil {
		s.Fatal("Failed to check LVM output: ", err)
	}
	if isEmpty {
		s.Fatal("LVM output is empty before reboot")
	}

	// Creating file ".default_key_stateful_migration" at path unencrypted.
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", "touch /mnt/stateful_partition/unencrypted/.default_key_stateful_migration")
	if err != nil {
		s.Fatal("Failed to create .default_key_stateful_migration file: ", err)
	}

	// Rebooting device.
	err = util.RebootDUT(ctx, dut)
	if err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	// Function to check LVM output.
	// Checking output after device reboot.
	isEmpty, err = checkLVMOutput(ctx, dut, s)
	if err != nil {
		s.Fatal("Failed to check LVM output: ", err)
	}
	if !isEmpty {
		s.Fatal("LVM output should be empty after reboot")
	}
}
