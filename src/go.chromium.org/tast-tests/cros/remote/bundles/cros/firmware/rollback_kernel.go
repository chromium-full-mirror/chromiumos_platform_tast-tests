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
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		Attr:         []string{"group:firmware"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
				Val:     common.BootModeNormal,
				Timeout: 30 * time.Minute,
			},
			{
				Name:      "dev",
				Fixture:   fixture.DevModeGBB,
				Val:       common.BootModeDev,
				Timeout:   10 * time.Minute,
				ExtraAttr: []string{"firmware_unstable"},
			},
		},
	})
}

func RollbackKernel(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	bootMode := s.Param().(common.BootMode)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Requiring servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require config: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Requiring KernelServiceClient: ", err)
	}

	kernAHostBackup, err := os.CreateTemp("", "KernABackup")
	if err != nil {
		s.Fatal("Failed to create temporary dir for kernel A backup")
	}
	kernBHostBackup, err := os.CreateTemp("", "KernBBackup")
	if err != nil {
		s.Fatal("Failed to create temporary dir for kernel B backup")
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
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

	needsRestoreFromDevMode := false
	rolledBackKernB := false

	s.Log("Backing up current DUT kernel copies")
	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}

	if bootMode == common.BootModeNormal {
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
		if needsRestoreFromDevMode {
			if err := bootToDevAndRestore(ctx, h, ms, rolledBackKernB); err != nil {
				s.Fatal("Failed to restore dut from dev mode: ", err)
			}
			needsRestoreFromDevMode = false
			rolledBackKernB = false
		}
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
	}(cleanupContext)

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	// Make sure we start with a deterministic state so we don't have a situation where
	// KERN-B is not bootable, additionally boot to both copies to make sure they are bootable.
	if err := bootToBothCopies(ctx, h, ms); err != nil {
		s.Fatal("Failed to make both copies bootable: ", err)
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	prevKernAVer, err := changeKernelVersion(ctx, h, pb.PartitionCopy_A, -1)
	if err != nil {
		s.Fatal("Failed to reduce KERN-A version by 1: ", err)
	}

	needsRestoreFromDevMode = true

	s.Log("Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.SkipWaitConnect); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if bootMode == common.BootModeDev {
		connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		if err := h.WaitConnect(connectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}

		needsRestoreFromDevMode = false

		if err := h.RequireKernelServiceClient(ctx); err != nil {
			s.Fatal("Failed to connect to kernel service: ", err)
		}

		s.Log("Verify DUT in ROOT-A")
		if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
			Copy: pb.PartitionCopy_A,
		}); err != nil {
			s.Fatal("Failed to verify DUT currently is in copy A: ", err)
		}

		s.Log("Get current kernel version for KERN-A and reset it")
		currVersion, err := changeKernelVersion(ctx, h, pb.PartitionCopy_A, +1)
		if err != nil {
			s.Fatal("Failed to reset KERN-A version: ", err)
		}

		if currVersion != prevKernAVer-1 {
			s.Fatalf("Expected kernel version to be %d but was %d", prevKernAVer-1, currVersion)
		}

		// Since no rollback is expected to occur in dev mode, the test ends here.
		return
	}

	connectCtx0, cancel0 := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel0()
	if err := h.WaitConnect(connectCtx0, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to connect to DUT and failed to boot to KERN-B: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in ROOT-B")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy B: ", err)
	}

	_, err = changeKernelVersion(ctx, h, pb.PartitionCopy_B, -1)
	if err != nil {
		s.Fatal("Failed to reduce KERN-B version by 1: ", err)
	}

	rolledBackKernB = true

	h.DisconnectDUT(ctx)
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Setting power state to reset failed: ", err)
	}

	// Waiting until keyboard is ready so we know we made it to recovery screen and recovery reason is set.
	if err := h.Servo.WaitFirmwareKeyboardNoCmd(ctx, h.Config.FirmwareScreen); err != nil {
		// If this fails, this is the same as sleeping for the waitTimeout
		testing.ContextLog(ctx, "Failed to wait for keyboard: ", err)
	}

	if err := bootToDevAndRestore(ctx, h, ms, rolledBackKernB); err != nil {
		s.Fatal("Failed to restore kernel versions from dev mode: ", err)
	}
	needsRestoreFromDevMode = false
	rolledBackKernB = false

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in ROOT-A")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to verify DUT currently is in copy A: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var recModeEvents = map[int64]string{}
		events, err := h.Reporter.EventlogList(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get event log")
		}
		// Case insensitive to match with different log messages.
		re := regexp.MustCompile(`(?i)recovery mode.*0x([a-fA-F0-9]+)`)
		for _, event := range events {
			if match := re.FindStringSubmatch(event.Message); match != nil {
				eventInt, err := strconv.ParseInt(match[1], 16, 64)
				if err != nil {
					s.Logf("Failed to parse %s as an int", match[1])
				}
				recModeEvents[eventInt] = event.Message
			}
		}

		foundExpRecReason := false
		if len(recModeEvents) == 0 {
			return errors.Errorf("expected recovery reason in eventlog but found none, got events: %v", events)
		}
		for _, recRes := range []int64{
			0x48, // No bootable disk found.
			0x5b, // No bootable kernel found on disk.
			0x43, // OS kernel failed signature check.
		} {
			if val, ok := recModeEvents[recRes]; ok {
				s.Logf("Found recovery reason 0x%x: %s", recRes, val)
				foundExpRecReason = true
				break
			}
		}
		if !foundExpRecReason {
			return errors.Errorf("Did not find expected recovery reasons in event log, found the following reasons for recovery instead: %v", recModeEvents)
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute,
	}); err != nil {
		s.Fatal("Looking for recovery reason: ", err)
	}
}

func bootToDevAndRestore(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, restoreB bool) error {

	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		testing.ContextLog(ctx, "Failed to set power_state:rec: ", err)
	}

	if err := ms.TriggerRecToDev(ctx); err != nil {
		return errors.Wrap(err, "failed to go from rec to dev mode")
	}

	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}

	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Could not determine boot device type: ", err)
	}
	if bootedFromRemovableDevice {
		testing.ContextLog(ctx, "DUT unexpectedly booted from the usb device")
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to kernel service")
	}

	testing.ContextLog(ctx, "Reset KERN-A version")
	_, err = changeKernelVersion(ctx, h, pb.PartitionCopy_A, +1)
	if err != nil {
		return errors.Wrap(err, "failed to increase KERN-A version by 1")
	}

	if restoreB {
		testing.ContextLog(ctx, "Reset KERN-B version")
		_, err = changeKernelVersion(ctx, h, pb.PartitionCopy_B, +1)
		if err != nil {
			testing.ContextLog(ctx, "Failed to increase KERN-B version by 1: ", err)
		}
	}

	testing.ContextLog(ctx, "Performing mode aware reboot to ensure boot to copy A")
	if err := ms.RebootToMode(ctx, common.BootModeNormal); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}

	h.DisconnectDUT(ctx)
	connectCtx1, cancel1 := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel1()
	if err := h.WaitConnect(connectCtx1); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}

	if err := h.SyncTastFilesToDUT(ctx); err != nil {
		return errors.Wrap(err, "copying Tast files to DUT failed")
	}

	return nil
}

func bootToBothCopies(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher) error {
	if _, err := h.KernelServiceClient.EnsureBothKernelCopiesBootable(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to ensure both kernel copies are bootable")
	}
	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		return errors.Wrap(err, "failed to prioritize KERN-B")
	}

	testing.ContextLog(ctx, "Performing mode aware reboot to ensure boot to copy B")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to kernel service")
	}

	testing.ContextLog(ctx, "Verify DUT in ROOT-B")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_B,
	}); err != nil {
		return errors.Wrap(err, "failed to verify DUT currently is in copy B")
	}

	if _, err := h.KernelServiceClient.PrioritizeKernelCopy(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		return errors.Wrap(err, "failed to prioritize KERN-A")
	}

	testing.ContextLog(ctx, "Performing mode aware reboot to ensure boot to copy A")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to kernel service")
	}

	testing.ContextLog(ctx, "Verify DUT in ROOT-A")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		return errors.Wrap(err, "failed to verify DUT currently is in copy A")
	}

	return nil
}

func changeKernelVersion(ctx context.Context, h *firmware.Helper, copy pb.PartitionCopy, change int) (int, error) {
	testing.ContextLog(ctx, "Get current kernel version")
	kernVersion, err := h.KernelServiceClient.GetKernelVersion(ctx, &pb.Partition{
		Name: pb.PartitionName_KERNEL,
		Copy: copy,
	})
	if err != nil {
		return -1, errors.Wrap(err, "failed to get kernel version")
	}
	testing.ContextLogf(ctx, "Current KERN-%s version is: %v", copy, kernVersion.Version)

	versionInt, err := strconv.Atoi(kernVersion.Version)
	if err != nil {
		return -1, errors.Wrap(err, "failed to parse kernel version as int")
	}
	newKernVersion := pb.KernelVersion{
		Version: strconv.Itoa(versionInt + change),
		Copy:    copy,
	}

	testing.ContextLogf(ctx, "Setting KERN-%s version to %s", copy, newKernVersion.Version)
	if _, err := h.KernelServiceClient.SetKernelVersion(ctx, &newKernVersion); err != nil {
		return versionInt, errors.Wrapf(err, "failed to set KERN-%s version to %s", copy, newKernVersion.Version)
	}

	return versionInt, nil
}
