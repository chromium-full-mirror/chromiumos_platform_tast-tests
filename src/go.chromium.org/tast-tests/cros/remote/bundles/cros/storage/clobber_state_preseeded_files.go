// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ClobberStatePreseededFiles,
		Desc: "Preservation of pre-seeded files",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func checkFileCount(ctx context.Context, dut *dut.DUT) (int, error) {

	powerWashCountFile := "/mnt/stateful_partition/unencrypted/preserve/powerwash_count"
	// Checking if powerwash_count exists at path /mnt/stateful_partition/unencrypted/preserve.
	exists, err := fileutils.HostFileExists(ctx, dut.Conn(), powerWashCountFile)
	if err != nil {
		return 0, errors.Wrap(err, "file powerwash_count could not be checked")
	} else if !exists {
		return 0, nil
	}

	// Reading count value from the file powerwash_count.
	cmd := fmt.Sprintf("cat %s", powerWashCountFile)
	out, err := util.RunCmdWithStringOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return 0, errors.Wrap(err, "unable to check the count for the file, err")
	}

	// Converting count string to integer.
	count, err := strconv.Atoi(out)
	if err != nil {
		return 0, errors.Errorf("unable to convert the count(%s) for the powerwash_count file, err %v ", out, err)
	}

	return count, nil
}

func ClobberStatePreseededFiles(ctx context.Context, s *testing.State) {

	dut := s.DUT()

	// Checking for powerwash count from the file powerwash_count.
	initialPowerWashCount, err := checkFileCount(ctx, dut)
	if err != nil {
		s.Fatal("Error while checking the count: ", err)
	}

	// Writing "fast safe keepimg" to /mnt/stateful_partition/factory_install_reset.
	cmd := fmt.Sprint(`echo "fast safe keepimg" > /mnt/stateful_partition/factory_install_reset`)
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		s.Fatal("Unable to write fast safe keepimg to factory_install_reset: ", err)
	}

	// Rebooting DUT.
	err = util.RebootDUT(ctx, dut)
	if err != nil {
		s.Fatal("Error while rebooting device: ", err)
	}

	// Checking for powerwash_count file count after reboot.
	updatedPowerWashCount, err := checkFileCount(ctx, dut)
	if err != nil {
		s.Fatal("Error while checking the content of file: ", err)
	}
	if updatedPowerWashCount != initialPowerWashCount+1 {
		s.Fatalf("Count of the powerwash_count file is not increased by 1 after the factory_install_reset and reboot, initialPowerWashCount:%d, updatedPowerWashCount:%d ", initialPowerWashCount, updatedPowerWashCount)
	}
}
