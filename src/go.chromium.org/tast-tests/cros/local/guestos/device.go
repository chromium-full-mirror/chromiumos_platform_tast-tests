// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/usbutil"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// FindUSBDeviceInVM checks if the target device is attached to the VM
func FindUSBDeviceInVM(ctx context.Context, vm *vm.VM, target *usbutil.Device) error {
	targetStr := fmt.Sprintf("ID %s:%s %s %s",
		target.VendorID, target.ProdID, target.VendorName, target.ProductName)

	output, err := vm.Command(ctx, "sudo", "lsusb").Output()
	if err != nil {
		return errors.Wrap(err, "failed to run lsusb")
	}

	res := strings.Split(string(output), "\n")
	for _, line := range res {
		if strings.Contains(line, targetStr) {
			return nil
		}
	}
	return errors.New("failed to find the device")
}

// VMStorageReadWriteTest checks if the contents written to and read from the device are same
func VMStorageReadWriteTest(ctx context.Context, vm *vm.VM, device, tempFilePath string, tempFileSizeInMB uint64) error {
	size := strconv.FormatUint(tempFileSizeInMB, 10)

	// Create a test content (all 0xff data) to be written to the device
	comm1 := vm.Command(ctx, "head", "-c", size+"m", "/dev/zero")
	comm2 := vm.Command(ctx, "tr", `"\0"`, `"\377"`)
	comm3 := vm.Command(ctx, "dd", "of="+tempFilePath)
	comm2.Stdin, _ = comm1.StdoutPipe()
	comm3.Stdin, _ = comm2.StdoutPipe()

	if err := comm1.Start(); err != nil {
		return errors.Wrap(err, "failed to create a temporary file")
	}
	if err := comm2.Start(); err != nil {
		return errors.Wrap(err, "failed to create a temporary file")
	}
	if err := comm3.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create a temporary file")
	}
	defer vm.Command(ctx, "rm", tempFilePath)

	// Write to the device
	if err := vm.Command(ctx, "sudo", "dd", "if="+tempFilePath, "of="+device, "oflag=direct").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to write to the device")
	}

	// Read from the device and calculate the hash of the read content
	comm1 = vm.Command(ctx, "sudo", "dd", "if="+device, "bs=1M", "count="+size)
	comm2 = vm.Command(ctx, "md5sum")
	comm2.Stdin, _ = comm1.StdoutPipe()
	if err := comm1.Start(); err != nil {
		return errors.Wrap(err, "failed to read from the device")
	}
	output, err := comm2.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to calculate the read hash")
	}
	hashDevice := strings.Split(string(output), " ")[0]

	// Compare the hash with that of the written content
	if output, err = vm.Command(ctx, "md5sum", tempFilePath).Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to calculate the written hash")
	}
	hashFile := strings.Split(string(output), " ")[0]

	if hashDevice != hashFile {
		return errors.Errorf("Read content and written content are inconsistent read: %s written: %s", hashDevice, hashFile)
	}

	return nil
}
