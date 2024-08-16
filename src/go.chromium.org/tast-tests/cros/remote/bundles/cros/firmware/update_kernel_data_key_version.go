// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
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
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		Data:         []string{kernelDataKeyVerMakekeyFile, kernelDataKeyVerCommonFile},
		Fixture:      fixture.DevModeGBB,
		Timeout:      30 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func UpdateKernelDataKeyVersion(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

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

	const (
		tempDir = "/usr/local/tmp/faft"
	)
	var (
		workDir = filepath.Join(tempDir, "autest")
		keysDir = filepath.Join(workDir, "keys")
	)

	s.Log("Backing up current Kernel")
	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

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

	// Make sure we start with a deterministic state so we don't have a
	// situation where for example KERN-B is many version ahead of KERN-A.
	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}

	defer func(ctx context.Context) {
		s.Log("Restoring kernel from backup")
		if _, err := h.KernelServiceClient.RestoreKernel(ctx, kernelBackup); err != nil {
			s.Fatal("Failed to restore kernel from backup: ", err)
		}
		s.Log("Deleting backup files from DUT")
		rmargs := []string{
			kernelBackup.KernA.BackupPath,
			kernelBackup.KernB.BackupPath,
		}
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", rmargs...).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete backup files: ", err)
		}
		s.Log("Ensuring both kernels are bootable after backup is completed")
		if _, err := h.KernelServiceClient.SetBothKernelBootable(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to set both kernel copies to bootable: ", err)
		}
		if err := warmResetToKernelPartition(ctx, h, ms, &pb.Partition{
			Name: pb.PartitionName_KERNEL,
			Copy: pb.PartitionCopy_B,
		}); err != nil {
			s.Fatal("Failed to prioritize KERN-B: ", err)
		}
		if _, err := h.KernelServiceClient.SetBothKernelBootable(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to set both kernel copies to bootable: ", err)
		}
		if err := warmResetToKernelPartition(ctx, h, ms, &pb.Partition{
			Name: pb.PartitionName_KERNEL,
			Copy: pb.PartitionCopy_A,
		}); err != nil {
			s.Fatal("Failed to prioritize KERN-A: ", err)
		}

		currVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
			Copy: pb.PartitionCopy_B,
		})
		if err != nil {
			s.Fatal("Failed to get data key version of KERN-B: ", err)
		}
		if currVersion.DataKeyVersion != initVersion.DataKeyVersion {
			s.Fatalf("Expected kernel version to be %s but was %s", initVersion.DataKeyVersion, currVersion.DataKeyVersion)
		}
		s.Logf("Rollback successful: current data key version of KERN-B is %s", currVersion.DataKeyVersion)
	}(cleanupContext)

	if err := warmResetToKernelPartition(ctx, h, ms, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-A: ", err)
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
	s.Log("Resigning the kernel to new version")
	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		s.Fatal("Failed to create futility instance: ", err)
	}
	signOptions := futility.
		NewSignKernelOptions(kernelBackup.KernB.Table.GetPartitionPath()).
		WithSignPrivatePath(filepath.Join(keysDir, "kernel_data_key.vbprivk")).
		WithKeyBlockPath(filepath.Join(keysDir, "kernel.keyblock")).
		WithOutputFile(filepath.Join(workDir, "output.bin"))
	if _, err = futilityInstance.SignKernel(ctx, signOptions); err != nil {
		s.Fatal("Failed to re-sign kernel: ", err)
	}

	args := []string{
		fmt.Sprintf("if=%s", filepath.Join(workDir, "output.bin")),
		fmt.Sprintf("of=%s", kernelBackup.KernB.Table.GetPartitionPath()),
		"conv=sync",
	}
	if err := h.DUT.Conn().CommandContext(ctx, "dd", args...).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to write new kernel: ", err)
	}

	s.Log("Prioritizing KERN-B to verify the update was successful")
	if _, err := h.KernelServiceClient.SetBothKernelBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to set both kernel copies to bootable: ", err)
	}

	if err := warmResetToKernelPartition(ctx, h, ms, &pb.Partition{
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
		s.Fatalf("Expected kernel version to be %s but was %s", newVersion.DataKeyVersion, currVersion.DataKeyVersion)
	}
	s.Logf("Successfully updated: current data key version of KERN-B is %s", currVersion.DataKeyVersion)
}

// warmResetToKernelPartition checks active kernel vs target, warm reboots DUT to target on mismatch.
func warmResetToKernelPartition(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, target *pb.Partition) error {
	currCopy, err := h.KernelServiceClient.GetCurrentCopy(ctx, &pb.Partition{})
	if err != nil {
		return errors.Wrap(err, "failed to get label of current kernel")
	}
	if currCopy.Copy != target.Copy {
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
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			return errors.Wrap(err, "failed to reboot")
		}
		if err := h.RequireKernelServiceClient(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to kernel service")
		}
		if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, target); err != nil {
			return errors.Wrapf(err, "failed to boot to KERN-%s", target.Copy.String())
		}
	} else {
		testing.ContextLog(ctx, "DUT has already booted to the expected kernel copy")
	}
	return nil
}
