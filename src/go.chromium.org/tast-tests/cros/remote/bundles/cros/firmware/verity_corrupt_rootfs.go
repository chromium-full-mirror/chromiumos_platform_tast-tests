// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VerityCorruptRootfs,
		Desc: "Verify corrupting kernel part a results in a boot to partition b",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

func VerityCorruptRootfs(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	initialCopy, err := h.KernelServiceClient.GetCurrentCopy(ctx, &pb.Partition{})
	if err != nil {
		s.Fatal("Failed to get label of initial part")
	}

	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}

	needsRestore := true

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if needsRestore {
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

	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}
	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to prioritize KERN-A: ", err)
	}

	s.Log("Performing mode aware reboot to ensure boot to copy A")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in KERN-A or ROOT-A")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy A: ", err)
	}

	// Backup verity hash after ensuring copy A is bootable as it may have changed after making it bootable.
	verityBackup, err := h.KernelServiceClient.BackupRootfsVerityHash(ctx, kernelBackup.KernA)
	if err != nil {
		s.Fatal("Failed to back up ROOT-A verity hash: ", err)
	}
	defer func(ctx context.Context) {
		if needsRestore {
			if err := h.RequireKernelServiceClient(ctx); err != nil {
				s.Fatal("Failed to connect to kernel service: ", err)
			}

			s.Log("Restoring ROOT-A verity hash from backup")
			if _, err := h.KernelServiceClient.RestoreRootfsVerityHash(ctx, verityBackup); err != nil {
				s.Fatal("Failed to restore rootfs verity hash from backup: ", err)
			}
		}
		s.Log("Delete verity backup file from DUT")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", verityBackup.BackupPath).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete backup files: ", err)
		}
	}(cleanupContext)

	s.Log("Corrupt ROOT-A verity hash")
	if _, err := h.KernelServiceClient.CorruptRootfsVerityHash(ctx, verityBackup); err != nil {
		s.Fatal("Failed to corrupt ROOT-A verity hash: ", err)
	}

	s.Log("Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in KERN-B or ROOT-B now")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy B: ", err)
	}

	if _, err := h.KernelServiceClient.RestoreRootfsVerityHash(ctx, verityBackup); err != nil {
		s.Fatal("Failed to restore rootfs verity hash from backup: ", err)
	}

	if _, err := h.KernelServiceClient.RestoreKernel(ctx, kernelBackup); err != nil {
		s.Fatal("Failed to restore kernel from backup: ", err)
	}

	s.Log("Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT booted to initial partition again")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, initialCopy); err != nil {
		// At this point both A and B are restored so it should boot to whatever it was booting to when backup was taken.
		s.Log("DUT was initially booted to different copy: ", err)
	}

	// Since we already restored verity hash and kernel from backup, and restored the
	// cgpt attributes successfully (set priority for A higher again), no need to restore again.
	needsRestore = false
}
