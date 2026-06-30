// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

var (
	kernelDataKeyVerMakekeyFile = "kernelDataKeyVer/make_keys.sh"
	kernelDataKeyVerCommonFile  = "kernelDataKeyVer/common.sh"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateKernelDataKeyVersion,
		Desc: "Validate the correct functionality of updating kernel data key versions",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		Data:         []string{kernelDataKeyVerMakekeyFile, kernelDataKeyVerCommonFile},
		Fixture:      fixture.DevModeGBB,
		Timeout:      30 * time.Minute,
	})
}

func UpdateKernelDataKeyVersion(ctx context.Context, s *testing.State) {
	const (
		tempDir    = "/usr/local/tmp/faft"
		devKeysDir = "/usr/share/vboot/devkeys"
	)
	var (
		keysDir = filepath.Join(tempDir, "autest/keys")
		h       = s.FixtValue().(*fixture.Value).Helper
	)
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}
	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Requiring KernelServiceClient: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	fs := dutfs.NewClient(h.RPCClient.Conn)
	if exist, err := fs.Exists(ctx, tempDir); err != nil {
		s.Fatal("Failed to check if temp dir is exist: ", err)
	} else if exist {
		if err := fs.RemoveAll(ctx, tempDir); err != nil {
			s.Fatal("Failed to remove temp dir: ", err)
		}
	}
	s.Log("Creating temp directories")
	if err := fs.MkDir(ctx, tempDir, 0777); err != nil {
		s.Fatal("Failed to make the temp directory: ", err)
	}
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := h.RequireRPCClient(ctx); err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		s.Log("Cleaning up temp directories")
		fs := dutfs.NewClient(h.RPCClient.Conn)
		if err := fs.RemoveAll(ctx, tempDir); err != nil {
			s.Fatal("Failed to remove temp dir: ", err)
		}
	}(cleanupContext)

	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}
	kernelNeedsRestore := true
	defer func(ctx context.Context) {
		if kernelNeedsRestore {
			if err := h.RequireKernelServiceClient(ctx); err != nil {
				s.Fatal("Failed to connect to kernel service: ", err)
			}
			s.Log("Restoring kernel from backup")
			if _, err := h.KernelServiceClient.RestoreKernel(ctx, kernelBackup); err != nil {
				s.Fatal("Failed to restore kernel from backup: ", err)
			}
			s.Log("Performing mode aware reboot to ensure restored kernel takes effect")
			if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
				s.Fatal("Failed to reboot: ", err)
			}
		}
		s.Log("Delete backup files from DUT")
		rmargs := []string{
			kernelBackup.KernA.BackupPath,
			kernelBackup.KernB.BackupPath,
		}
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", rmargs...).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete backup files: ", err)
		}
	}(cleanupContext)

	// Make sure we start with a deterministic state so we don't have a situation where
	// KERN-B is not bootable, additionally boot to both copies to make sure they are bootable.
	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}
	if err := coldResetToKernelPartition(ctx, h, ms, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-B: ", err)
	}
	if err := coldResetToKernelPartition(ctx, h, ms, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-A: ", err)
	}

	s.Log("Getting original data key version of KERN-B")
	initVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_B,
	})
	if err != nil {
		s.Fatal("Failed to get data key version of KERN-B: ", err)
	}
	s.Log("Original data key version of KERN-B is: ", initVersion.DataKeyVersion)

	versionInt, err := strconv.Atoi(initVersion.DataKeyVersion)
	if err != nil {
		s.Fatal("Failed to parse kernel version as int")
	}
	newVersion := pb.KernelVersion{
		DataKeyVersion: strconv.Itoa(versionInt + 1),
		Copy:           pb.PartitionCopy_B,
	}
	s.Log("Setting data key version of KERN-B to ", newVersion.DataKeyVersion)
	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}

	s.Log("Preparing the key files that are going to be resigned")
	if err := h.PrepareKeysWithScript(ctx, firmware.MakeKeysOption{
		VersionToSign: newVersion.DataKeyVersion,
		// Copies original key files from the DUT to the KeysDir
		KeysDir:        keysDir,
		MakeKeyFileDir: kernelDataKeyVerMakekeyFile,
		CommonFileDir:  kernelDataKeyVerCommonFile,
		//  Absolute paths for the shell scripts in firmware/data
		ShellScript: s.DataPaths(),
	}); err != nil {
		s.Fatal("Failed to prepare the key files: ", err)
	}
	s.Log("Resigning KERN-B with new keys")
	if err := resignKernelBWithKeys(ctx, h, keysDir); err != nil {
		s.Fatal("Fail to resign KERN-B to new version: ", err)
	}
	s.Log("Prioritizing KERN-B to verify the update was successful")
	if _, err := h.KernelServiceClient.SetBothKernelBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to set both kernel copies to bootable: ", err)
	}
	if err := coldResetToKernelPartition(ctx, h, ms, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-B: ", err)
	}

	s.Log("Getting current data key version of KERN-B")
	currVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_B,
	})
	if err != nil {
		s.Fatal("Failed to get data key version of KERN-B: ", err)
	}
	if currVersion.DataKeyVersion != newVersion.DataKeyVersion {
		s.Fatalf("Expected kernel version to be %s but got %s", newVersion.DataKeyVersion, currVersion.DataKeyVersion)
	}
	s.Logf("Successfully updated: current data key version of KERN-B is %s", currVersion.DataKeyVersion)

	s.Log("Resigning KERN-B with orignal keys")
	if err := resignKernelBWithKeys(ctx, h, devKeysDir); err != nil {
		s.Fatal("Fail to resign KERN-B to orignal version: ", err)
	}
	if _, err := h.KernelServiceClient.SetBothKernelBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to set both kernel copies to bootable: ", err)
	}
	if err := coldResetToKernelPartition(ctx, h, ms, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-B: ", err)
	}
	if _, err := h.KernelServiceClient.SetBothKernelBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to set both kernel copies to bootable: ", err)
	}
	if err := coldResetToKernelPartition(ctx, h, ms, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-A: ", err)
	}
	rollbackVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_B,
	})
	if err != nil {
		s.Fatal("Failed to get data key version of KERN-B: ", err)
	}
	if rollbackVersion.DataKeyVersion != initVersion.DataKeyVersion {
		s.Fatalf("Expected kernel version to be %s but was %s", initVersion.DataKeyVersion, rollbackVersion.DataKeyVersion)
	}
	s.Logf("Rollback successful: current data key version of KERN-B is %s", rollbackVersion.DataKeyVersion)
	kernelNeedsRestore = false
}

// coldResetToKernelPartition checks active kernel vs target, reboots DUT to target on mismatch.
func coldResetToKernelPartition(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, target *pb.Partition) error {
	testing.ContextLog(ctx, "Sleeping for 10s")
	// GoBigSleepLint: There is a risk that the priority value may revert to its
	// original setting if the priority is set immediately after
	// EnsureBothKernelCopiesBootable() and SetBothKernelBootable().
	// Add a 10-second delay before setting the priority.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for 10 seconds")
	}
	testing.ContextLog(ctx, "Rebooting DUT to KERN-", target.Copy.String())
	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, target); err != nil {
		return errors.Wrapf(err, "failed to prioritize KERN-%s", target.Copy.String())
	}
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	if err := h.RequireKernelServiceClient(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to kernel service")
	}
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, target); err != nil {
		return errors.Wrapf(err, "failed to boot to KERN-%s", target.Copy.String())
	}
	return nil
}

// resignKernelBWithKeys resign kernel B with keys in keysDir.
func resignKernelBWithKeys(ctx context.Context, h *firmware.Helper, keysDir string) error {
	label := "KERN-B"
	tmpFile, err := os.CreateTemp("/var/tmp", fmt.Sprintf("%s-repack_*.bin", label))
	if err != nil {
		os.Remove(tmpFile.Name())
		return errors.Wrap(err, "failed to create tmpfile for storing modified kernel")
	}
	defer os.Remove(tmpFile.Name())
	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		return errors.Wrap(err, "failed to create futility instance")
	}
	blockDevice, err := h.KernelServiceClient.GetCurrentRootDevice(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get root device")
	}
	GetCgptTable, err := h.KernelServiceClient.GetCgptTable(ctx, &pb.GetCgptTableRequest{
		BlockDevice: blockDevice.RootDev,
	})
	if err != nil {
		return errors.Wrap(err, "failed to read cgpt table")
	}
	partitionInfo, ok := GetCgptTable.CgptTable[label]
	if !ok {
		return errors.Errorf("no partition info found for label %s", label)
	}
	partitionPath := partitionInfo.GetPartitionPath()
	if partitionPath == "" {
		return errors.Errorf("partition path for label %s is empty", label)
	}
	signOptions := futility.
		NewSignKernelOptions(partitionPath).
		WithSignPrivatePath(filepath.Join(keysDir, "kernel_data_key.vbprivk")).
		WithKeyBlockPath(filepath.Join(keysDir, "kernel.keyblock")).
		WithOutputFile(tmpFile.Name())
	if _, err = futilityInstance.SignKernel(ctx, signOptions); err != nil {
		return errors.Wrap(err, "failed to re-sign kernel using futility instance")
	}
	args := []string{
		fmt.Sprintf("if=%s", tmpFile.Name()),
		fmt.Sprintf("of=%s", partitionPath),
		"conv=noerror,sync",
	}
	if err := h.DUT.Conn().CommandContext(ctx, "dd", args...).Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to write new kernel")
	}
	return nil
}
