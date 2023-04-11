// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	pb "chromiumos/tast/services/cros/firmware"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptKernelAB,
		Desc: "Verify corrupting kernel part results in a boot to other partition",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "a",
				Fixture: fixture.NormalMode,
				Val:     pb.PartitionCopy_A,
			},
			{
				Name:    "a_dev",
				Fixture: fixture.DevModeGBB,
				Val:     pb.PartitionCopy_A,
			},
			{
				Name:    "b",
				Fixture: fixture.NormalMode,
				Val:     pb.PartitionCopy_B,
			},
			{
				Name:    "b_dev",
				Fixture: fixture.DevModeGBB,
				Val:     pb.PartitionCopy_B,
			},
		},
	})
}

func CorruptKernelAB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	kernCopy := s.Param().(pb.PartitionCopy)

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

	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := h.RequireKernelServiceClient(ctx); err != nil {
			s.Fatal("Failed to connect to kernel service: ", err)
		}

		s.Log("Restoring kernel from backup")
		if _, err := h.KernelServiceClient.RestoreKernel(ctx, kernelBackup); err != nil {
			s.Fatal("Failed to restore kernel from backup: ", err)
		}

		s.Log("Performing mode aware reboot to ensure restored kernel takes effect")
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to reboot: ", err)
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

	kernCopyStr := "A"
	notKernCopyStr := "B"
	notKernCopy := pb.PartitionCopy_B
	copyToCorrupt := kernelBackup.KernA
	if kernCopy == pb.PartitionCopy_B {
		kernCopyStr = "B"
		notKernCopyStr = "A"
		notKernCopy = pb.PartitionCopy_A
		copyToCorrupt = kernelBackup.KernB
	}

	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: kernCopy,
	}); err != nil {
		s.Fatalf("Failed to prioritize copy %q: %v", kernCopyStr, err)
	}

	backupPrioritizedCopy, err := h.KernelServiceClient.BackupPartition(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: kernCopy,
	})
	if err != nil {
		s.Fatalf("Failed to back up KERN-%s: %v", kernCopyStr, err)
	}

	s.Log("Performing mode aware reboot to ensure boot to copy ", kernCopyStr)
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy ", kernCopyStr)
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: kernCopy,
	}); err != nil {
		s.Fatalf("Failed to verify DUT currently is in copy %q: %v", kernCopyStr, err)
	}

	s.Log("Corrupt kernel partition ", copyToCorrupt.Table.Label)
	if _, err := h.KernelServiceClient.CorruptKernel(ctx, copyToCorrupt); err != nil {
		s.Fatalf("Failed to corrupt %s: %v", copyToCorrupt.Table.Label, err)
	}

	s.Log("Performing mode aware reboot to ensure boot copy ", notKernCopyStr)
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy ", notKernCopyStr)
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: notKernCopy,
	}); err != nil {
		s.Fatalf("Failed to verify DUT currently is in copy %q: %v", notKernCopyStr, err)
	}

	s.Logf("Restoring just KERN-%s from backup", kernCopyStr)
	if _, err := h.KernelServiceClient.RestorePartition(ctx, backupPrioritizedCopy); err != nil {
		s.Fatalf("Failed to restore KERN-%s from backup: %v", kernCopyStr, err)
	}

	s.Log("Performing mode aware reboot to ensure boots back to copy ", kernCopyStr)
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy ", kernCopyStr)
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: kernCopy,
	}); err != nil {
		s.Fatalf("Failed to verify DUT currently is in copy %q: %v", kernCopyStr, err)
	}
}
