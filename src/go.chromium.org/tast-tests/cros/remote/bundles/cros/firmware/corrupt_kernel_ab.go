// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/tbdep"
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
		Func: CorruptKernelAB,
		Desc: "Verify corrupting kernel part results in a boot to other partition",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      30 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "a",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.NormalMode,
				Val:       pb.PartitionCopy_A,
			},
			{
				Name:    "a_dev",
				Fixture: fixture.DevModeGBB,
				Val:     pb.PartitionCopy_A,
			},
			{
				Name:      "b",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.NormalMode,
				Val:       pb.PartitionCopy_B,
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
	copyToCorrupt := s.Param().(pb.PartitionCopy)

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
			h.DisconnectDUT(ctx)
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

	kernToCorrupt := kernelBackup.KernA.Table
	otherCopy := pb.PartitionCopy_B
	if copyToCorrupt == pb.PartitionCopy_B {
		kernToCorrupt = kernelBackup.KernB.Table
		otherCopy = pb.PartitionCopy_A
	}

	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}
	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: copyToCorrupt,
	}); err != nil {
		s.Fatalf("Failed to prioritize copy %s: %v", copyToCorrupt, err)
	}

	s.Log("Performing mode aware reboot to ensure boot to copy ", copyToCorrupt)
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy ", copyToCorrupt)
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: copyToCorrupt,
	}); err != nil {
		s.Fatalf("Failed to prioritize copy %s: %v", copyToCorrupt, err)
	}

	if _, err := h.KernelServiceClient.SetKernelHeaderMagic(ctx, &pb.KernelHeaderMagicInfo{
		Name:  pb.PartitionName_KERNEL,
		Copy:  copyToCorrupt,
		Magic: pb.KernelHeaderMagic_CORRUPTD,
	}); err != nil {
		s.Fatalf("Failed to corrupt %s: %v", kernToCorrupt.Label, err)
	}

	s.Log("Performing mode aware reboot to ensure boot copy ", otherCopy)
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy ", otherCopy)
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: otherCopy,
	}); err != nil {
		s.Fatalf("Failed to verify DUT currently is in copy %s: %v", otherCopy, err)
	}

	s.Log("Restore kernel partition ", kernToCorrupt.Label)
	if _, err = h.KernelServiceClient.SetKernelHeaderMagic(ctx, &pb.KernelHeaderMagicInfo{
		Name:      pb.PartitionName_KERNEL,
		Copy:      copyToCorrupt,
		Magic:     pb.KernelHeaderMagic_CHROMEOS,
		ForceBoot: true,
	}); err != nil {
		s.Fatalf("Failed to restore %s: %v", kernToCorrupt.Label, err)
	}

	s.Log("Performing mode aware reboot to ensure boots back to copy ", copyToCorrupt)
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy ", copyToCorrupt)
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: copyToCorrupt,
	}); err != nil {
		s.Fatalf("Failed to verify DUT currently is in copy %s: %v", copyToCorrupt, err)
	}

	needsRestore = false
}
