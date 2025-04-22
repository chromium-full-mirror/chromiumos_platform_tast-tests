// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// BootFromUSB function will set the crossystem dev_default to usb.
func BootFromUSB(ctx context.Context, dut *dut.DUT) error {

	// Enabling boot from usb.
	_, err := RunCmdWithOutput(ctx, dut, "sh", "-c", "crossystem dev_default_boot=usb")
	if err != nil {
		return errors.Wrap(err, "error in enabling boot from usb")
	}

	// Rebooting device.
	err = dut.Reboot(ctx)
	if err != nil {
		return errors.Wrap(err, "error in rebooting the device")
	}

	return nil
}

// InstallChromeOS function will install the ChromeOS using the usb drive.
func InstallChromeOS(ctx context.Context, dut *dut.DUT) error {

	// chromeOS-install.
	_, err := RunCmdWithOutput(ctx, dut, "sh", "-c", "chromeos-install --skip_postinstall -y")
	if err != nil {
		return errors.Wrap(err, "failed to install ChromeOS")
	}

	// Get stateful partition path.
	partition, err := GetStatefulPartition(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get statefulDev")
	}
	// Using partition 11 to format.
	partition += "1"

	// Formatting partition with ext4.
	cmd := fmt.Sprintf("mkfs.ext4 %v", partition)
	_, err = RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to format the partition")
	}

	// Enabling boot from disk.
	_, err = RunCmdWithOutput(ctx, dut, "sh", "-c", "crossystem dev_default_boot=disk")
	if err != nil {
		return errors.Wrap(err, "error in enabling boot from disk")
	}
	// Rebooting device.
	err = dut.Reboot(ctx)
	if err != nil {
		return errors.Wrap(err, "error in rebooting the device")
	}

	return nil
}