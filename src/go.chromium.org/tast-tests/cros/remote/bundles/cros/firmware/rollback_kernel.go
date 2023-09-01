// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RollbackKernel,
		Desc: "Decrement both kernel copies version and verify its goes to recovery",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Params: []testing.Param{
			{
				Name:      "normal",
				Fixture:   fixture.NormalMode,
				Val:       common.BootModeNormal,
				ExtraAttr: []string{"firmware_usb"},
				Timeout:   20 * time.Minute,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
				Val:     common.BootModeDev,
				Timeout: 10 * time.Minute,
			},
		},
	})
}

func RollbackKernel(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	bootMode := s.Param().(common.BootMode)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Requiring KernelServiceClient: ", err)
	}

	// Keep the rootdev for the disk to pass explicitly so as to not accidentally
	// use/modify copies from the USB instead.
	diskRootDev, err := h.KernelServiceClient.GetCurrentRootDevice(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get rootdev for disk: ", err)
	}

	kernAHostBackup, err := os.CreateTemp("", "KernABackup")
	if err != nil {
		s.Fatal("Failed to create temporary dir for kernel A backup")
	}
	kernBHostBackup, err := os.CreateTemp("", "KernBBackup")
	if err != nil {
		s.Fatal("Failed to create temporary dir for kernel B backup")
	}

	defer func(ctx context.Context) {
		kernAHostBackup.Close()
		kernBHostBackup.Close()
		if err := os.Remove(kernAHostBackup.Name()); err != nil {
			s.Log("Failed to delete KERN-A back up dir from host")
		}
		if err := os.Remove(kernBHostBackup.Name()); err != nil {
			s.Log("Failed to delete KERN-B back up dir from host")
		}
	}(cleanupContext)

	s.Log("Backing up current DUT kernel copies")
	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{
		RootDev: diskRootDev.RootDev,
	})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}

	if bootMode == common.BootModeNormal {
		skipFlashUSB := false
		if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
			var err error
			skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
			if err != nil {
				s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
			}
		}
		cs := s.CloudStorage()
		if skipFlashUSB {
			cs = nil
		}
		if err := h.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("USBKey not working: ", err)
		}

		if !h.DoesServerHaveTastHostFiles() {
			if err := h.CopyTastFilesFromDUT(ctx); err != nil {
				s.Fatal("Copying Tast files to Host failed: ", err)
			}
		}
		s.Log("Copying kernel back up to host")
		if err := linuxssh.GetFile(ctx, h.DUT.Conn(), kernelBackup.KernA.BackupPath, kernAHostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to copy a KERN-A backup to the host")
		}
		if err := linuxssh.GetFile(ctx, h.DUT.Conn(), kernelBackup.KernB.BackupPath, kernBHostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to copy a KERN-B backup to the host")
		}
		s.Logf("Backed up KERN-A to %q and KERN-B to %q on host", kernAHostBackup.Name(), kernBHostBackup.Name())
	}

	defer func(ctx context.Context) {
		if bootMode == common.BootModeNormal {
			s.Log("Sync KERN-A/B backups from host to DUT")
			if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{
				kernAHostBackup.Name(): kernelBackup.KernA.BackupPath,
				kernBHostBackup.Name(): kernelBackup.KernB.BackupPath,
			}, linuxssh.DereferenceSymlinks); err != nil {
				s.Fatal("Failed to get backup files to DUT from host")
			}
		}

		if err := h.RequireKernelServiceClient(ctx); err != nil {
			s.Error("Failed to connect to kernel service: ", err)
		}

		s.Log("Restoring kernel from backup")
		if _, err := h.KernelServiceClient.RestoreKernel(ctx, kernelBackup); err != nil {
			s.Error("Failed to restore kernel from backup: ", err)
		}

		s.Log("Delete backup files from DUT")
		rmargs := []string{
			kernelBackup.KernA.BackupPath,
			kernelBackup.KernB.BackupPath,
		}
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", rmargs...).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete backup files: ", err)
		}

		s.Log("Performing mode aware reboot to ensure restored boot from disk")
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	}(cleanupContext)

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	// Make sure we start with a deterministic state so we don't have a
	// situation where for example KERN-B is not bootable.
	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &pb.Partition{}); err != nil {
		s.Fatal("Failed to ensure both kernel copies are bootable: ", err)
	}
	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
		RootDev: diskRootDev.RootDev,
		Name:    pb.PartitionName_KERNEL,
		Copy:    pb.PartitionCopy_A,
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

	prevKernAVer, err := changeKernelVersion(ctx, h, diskRootDev.RootDev, pb.PartitionCopy_A, -1)
	if err != nil {
		s.Fatal("Failed to reduce KERN-A version by 1: ", err)
	}

	s.Log("Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	if bootMode == common.BootModeDev {
		s.Log("Verify DUT in KERN-A or ROOT-A and booted from disk")
		if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
			RootDev: diskRootDev.RootDev,
			Copy:    pb.PartitionCopy_A,
		}); err != nil {
			s.Fatal("Failed to verify DUT currently is in copy A: ", err)
		}

		s.Log("Get current kernel version for KERN-A")
		currKernAVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
			Name: pb.PartitionName_KERNEL,
			Copy: pb.PartitionCopy_A,
		})
		if err != nil {
			s.Fatal("Failed to get kernel version: ", err)
		}
		s.Log("Current kernel version is: ", currKernAVersion.Version)

		if currKernAVersion.Version != strconv.Itoa(prevKernAVer-1) {
			s.Fatalf("Expected kernel version to be %d but was %s", prevKernAVer-1, currKernAVersion.Version)
		}

		// Since no rollback is expected to occur in dev mode, the test ends here.
		return
	}

	s.Log("Verify DUT in KERN-B or ROOT-B")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		RootDev: diskRootDev.RootDev,
		Copy:    pb.PartitionCopy_B,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy B: ", err)
	}

	var previousEvent reporters.Event
	oldEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		s.Fatal("Finding most recent event: ", err)
	}
	if len(oldEvents) > 0 {
		previousEvent = oldEvents[len(oldEvents)-1]
	}

	_, err = changeKernelVersion(ctx, h, diskRootDev.RootDev, pb.PartitionCopy_B, -1)
	if err != nil {
		s.Fatal("Failed to reduce KERN-B version by 1: ", err)
	}

	if err := ms.WarmResetToRecovery(ctx, bootMode, firmware.CopyTastFiles); err != nil {
		s.Fatal("Failed to perform recovery boot to USB")
	}

	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Error("Could not determine boot device type: ", err)
	}
	if !bootedFromRemovableDevice {
		s.Error("DUT unexpectedly did not boot from the usb device")
	}

	// Note: must enable rpc service after BootedFromRemovableDevice, doing it before causes the rpc service to stop for some reason.
	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Reset KERN-A version")
	_, err = changeKernelVersion(ctx, h, diskRootDev.RootDev, pb.PartitionCopy_A, +1)
	if err != nil {
		s.Error("Failed to increase KERN-A version by 1: ", err)
	}

	s.Log("Reset KERN-B version")
	_, err = changeKernelVersion(ctx, h, diskRootDev.RootDev, pb.PartitionCopy_B, +1)
	if err != nil {
		s.Error("Failed to increase KERN-B version by 1: ", err)
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
		RootDev: diskRootDev.RootDev,
		Copy:    pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy A: ", err)
	}

	var recModeEvents = map[int64]string{}

	events, err := h.Reporter.EventlogListAfter(ctx, previousEvent)
	if err != nil {
		s.Fatal("Failed to get event log: ", err)
	}
	// Case insensitive to match with different log messages.
	re := regexp.MustCompile(`(?i)recovery mode.*0x([a-fA-F0-9]+)`)
	for _, event := range events {
		if match := re.FindStringSubmatch(event.Message); match != nil {
			eventInt, err := strconv.ParseInt(match[1], 16, 64)
			if err != nil {
				s.Errorf("Failed to parse %s as an int", match[1])
			}
			recModeEvents[eventInt] = event.Message
		}
	}

	if len(recModeEvents) == 0 {
		s.Fatal("Expected recovery reason in eventlog but found none, got events: ", events)
	}
	for _, recRes := range []int64{
		0x48, // No bootable disk found.
		0x5b, // No bootable kernel found on disk.
		0x43, // OS kernel failed signature check.
	} {
		if val, ok := recModeEvents[recRes]; ok {
			s.Logf("Found recovery reason %x: %s", recRes, val)
			return
		}
	}

	s.Fatal("Did not find expected recovery reasons in event log, found the following reasons for recovery instead: ", recModeEvents)
}

func changeKernelVersion(ctx context.Context, h *firmware.Helper, rootdev string, copy pb.PartitionCopy, change int) (int, error) {
	testing.ContextLog(ctx, "Get current kernel version")
	kernVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		RootDev: rootdev,
		Name:    pb.PartitionName_KERNEL,
		Copy:    copy,
	})
	if err != nil {
		return -1, errors.Wrap(err, "failed to get kernel version")
	}
	testing.ContextLogf(ctx, "Current KERN-%s version is: %v", copy, kernVersion.Version)

	newKernVersion := kernVersion
	versionInt, err := strconv.Atoi(kernVersion.Version)
	if err != nil {
		return -1, errors.Wrap(err, "failed to parse kernel version as int")
	}
	newKernVersion.Version = strconv.Itoa(versionInt + change)

	testing.ContextLogf(ctx, "Setting KERN-%s version to %s", copy, newKernVersion.Version)
	if _, err := h.KernelServiceClient.SetKernelVersion(ctx, newKernVersion); err != nil {
		return versionInt, errors.Wrapf(err, "failed to set KERN-%s version to %d", copy, versionInt)
	}

	return versionInt, nil
}
