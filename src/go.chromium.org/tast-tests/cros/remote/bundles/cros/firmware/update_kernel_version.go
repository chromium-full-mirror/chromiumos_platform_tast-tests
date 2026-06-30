// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateKernelVersion,
		Desc: "Update kernel version bits in CGPT and verify its consistency",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		Fixture:      fixture.DevModeGBB,
		Timeout:      15 * time.Minute,
	})
}

// UpdateKernelVersion reads CGPT headers of kernel partition and
// then modifies the version bits to mimic kernel update and checks
// if it persist after reboot
func UpdateKernelVersion(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Requiring KernelServiceClient: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	s.Log("Backing up current Kernel")
	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{})
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
		s.Log("Delete backup files from DUT")
		rmargs := []string{
			kernelBackup.KernA.BackupPath,
			kernelBackup.KernB.BackupPath,
		}
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", rmargs...).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete backup files: ", err)
		}

		s.Log("Performing mode aware reboot to ensure restored kernel takes effect")
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	}(cleanupContext)

	// Make sure we start with a deterministic state so we don't have a
	// situation where for example KERN-B is many version ahead of KERN-A.
	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}
	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
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

	s.Log("Get initial kernel version for KERN-A")
	initVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	})
	if err != nil {
		s.Fatal("Failed to get kernel version: ", err)
	}
	s.Log("Initial kernel version is: ", initVersion.Version)

	versionInt, err := strconv.Atoi(initVersion.Version)
	if err != nil {
		s.Fatal("Failed to parse kernel version as int")
	}
	newVersion := pb.KernelVersion{
		Version: strconv.Itoa(versionInt + 1),
		Copy:    pb.PartitionCopy_A,
	}

	s.Log("Setting KERN-A version to ", newVersion.Version)
	if _, err := h.KernelServiceClient.SetKernelVersion(ctx, &newVersion); err != nil {
		s.Fatal("Failed to set kernel version: ", err)
	}

	s.Log("Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Get current kernel version for KERN-A")
	currVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	})
	if err != nil {
		s.Fatal("Failed to get kernel version: ", err)
	}
	s.Log("Current kernel version is: ", currVersion.Version)

	if currVersion.Version != newVersion.Version {
		s.Fatalf("Expected kernel version to be %s but was %s", newVersion.Version, currVersion.Version)
	}

	s.Log("Verify DUT in KERN-A or ROOT-A")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy A: ", err)
	}
}
