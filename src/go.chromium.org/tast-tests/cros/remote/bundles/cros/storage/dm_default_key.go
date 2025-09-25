// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	unencryptedPath = "/mnt/stateful_partition/unencrypted/"
	truncateFile    = unencryptedPath + "test"
	testDeviceDir   = unencryptedPath + "test_device"
	testDMFile      = testDeviceDir + "/test_file"
	dmDevice        = "/dev/mapper/sample_device"
)

var (
	loopDevice = ""
	key        = ""
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DMDefaultKey,
		Desc: "Validation for dm-default-key",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func syncAndRebootDUT(ctx context.Context, dut *dut.DUT) error {

	// Syncing.
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", "sync")
	if err != nil {
		return errors.Wrap(err, "failed to sync")
	}

	// Rebooting device.
	err = util.RebootDUT(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "error while rebooting device")
	}
	return nil
}

func mountDMDevice(ctx context.Context, dut *dut.DUT) error {

	cmd := fmt.Sprintf(`mount %s %s`, dmDevice, testDeviceDir)
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to mount the device")
	}
	return nil
}

func createDirAndMountDevice(ctx context.Context, dut *dut.DUT) error {

	// Creating the test_device dir inside unencrypted.
	cmd := fmt.Sprintf(`mkdir -p %s`, testDeviceDir)
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create dir")
	}

	// Mounting the device.
	err = mountDMDevice(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to mount the device")
	}

	return nil
}

func createDMDevice(ctx context.Context, dut *dut.DUT) error {

	// Create dmsetup table.
	tablecmd := fmt.Sprintf(`0 $(blockdev --getsz "%s") default-key aes-xts-plain64 %s 0 %s 0 1 allow_discards`, loopDevice, key, loopDevice)
	cmd := fmt.Sprintf(`dmsetup create sample_device --table "%s"`, tablecmd)
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to create dmsetup")
	}
	return nil
}

func attachLoopDevice(ctx context.Context, dut *dut.DUT) error {

	// Running losetup command.
	cmd := fmt.Sprintf("losetup -f %s --show", truncateFile)
	out, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to run losetup command")
	}
	loopDevice = strings.TrimSpace(string(out))
	return nil
}

func createEncryptedLoopDevice(ctx context.Context, dut *dut.DUT) error {

	// This is to create a 1Gb test file.
	cmd := fmt.Sprintf("truncate -s 1G %s", truncateFile)
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to run truncate command")
	}

	// This is to attach the loop device.
	err = attachLoopDevice(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to attach loop device")
	}

	// Generating key and storing it to key var.
	out, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", "tr -dc a-f0-9 </dev/urandom | dd bs=128 count=1 2>/dev/null")
	if err != nil {
		return errors.Wrap(err, "failed to generate key")
	}
	key = strings.TrimSpace(string(out))

	// This function will create the dm device.
	err = createDMDevice(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to create dm device")
	}

	// Formatting the dm device.
	cmd = fmt.Sprintf(`mkfs.ext4 %s`, dmDevice)
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to format device")
	}

	// This function will mount the device.
	err = createDirAndMountDevice(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to create dir or mount the device")
	}

	return nil
}

func removeEntries(ctx context.Context, dut *dut.DUT) {

	util.RunCmdWithOutput(ctx, dut, "sh", "-c", "umount", testDeviceDir)
	util.RunCmdWithOutput(ctx, dut, "sh", "-c", "dmsetup", "remove", "sample_device")
	util.RunCmdWithOutput(ctx, dut, "sh", "-c", "losetup", "-d", loopDevice)
	os.Remove(truncateFile)
}

func DMDefaultKey(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	hasDefaultKey := util.HasDefaultKeyStatefulDiskLayout(ctx, dut)
	if !hasDefaultKey {
		s.Log("Skipping test: DUT does not have default-key-stateful disk layout")
		return
	}

	// This is to remove all created file and images after the testing.
	defer removeEntries(ctx, dut)

	// This function will create the 1Gb truncate file, attach the loop device, create dm and mount the device.
	err := createEncryptedLoopDevice(ctx, dut)
	if err != nil {
		s.Fatal("Error while creating encrypted loop device: ", err)
	}

	// Writing the data to the file.
	var content string = "foobar"
	cmd := fmt.Sprintf(`echo %s > %s`, content, testDMFile)
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		s.Fatal("Failed to write data to the file: ", err)
	}

	// This function will sync the file and after that reboot the DUT.
	err = syncAndRebootDUT(ctx, dut)
	if err != nil {
		s.Fatal("Error while syncing or rebooting the dut: ", err)
	}
	// Re-attaching the loop device after reboot.
	err = attachLoopDevice(ctx, dut)
	if err != nil {
		s.Fatal("Error while attaching the loop device after reboot: ", err)
	}

	// Again creating the dm device after reboot.
	err = createDMDevice(ctx, dut)
	if err != nil {
		s.Fatal("Error in creating the dm device after reboot: ", err)
	}
	// Remounting the dm device.
	err = mountDMDevice(ctx, dut)
	if err != nil {
		s.Fatal("Error in mounting the dm device after reboot: ", err)
	}

	// This function is to validate the file content.
	err = util.VerifyFileContent(ctx, dut, testDMFile, content)
	if err != nil {
		s.Fatal("Error while verifying the file content: ", err)
	}
}
