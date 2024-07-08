// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptBothKernelCopies,
		Desc: "Verify corrupting both kernel part results in a recovery boot",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		Vars:         []string{"firmware.skipFlashUSB"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      2 * time.Hour,
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func CorruptBothKernelCopies(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper

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

	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Setting usb mux state to off: ", err)
	}

	kernelBackup, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{})
	if err != nil {
		s.Fatal("Failed to back up KERN-A and KERN-B: ", err)
	}

	// Save KERN-A/B backups to servo.
	kernAHostBackup, err := os.CreateTemp("", "KernABackup")
	if err != nil {
		s.Fatal("Failed to create temporary file for kernel A backup")
	}
	kernBHostBackup, err := os.CreateTemp("", "KernBBackup")
	if err != nil {
		s.Fatal("Failed to create temporary file for kernel B backup")
	}
	s.Logf("Backed up KERN-A to %q and KERN-B to %q on host", kernAHostBackup.Name(), kernBHostBackup.Name())
	defer func() {
		kernAHostBackup.Close()
		kernBHostBackup.Close()
		os.Remove(kernAHostBackup.Name())
		os.Remove(kernBHostBackup.Name())
	}()

	s.Log("Copying kernel back up to host")
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), kernelBackup.KernA.BackupPath, kernAHostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy a KERN-A backup to the host")
	}
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), kernelBackup.KernB.BackupPath, kernBHostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy a KERN-B backup to the host")
	}

	s.Log("Copying tast files from DUT to host")
	if err := h.CopyTastFilesFromDUT(ctx); err != nil {
		s.Log(err, "copy files from DUT")
	}
	needsRestore := true

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if needsRestore {
			if err := h.WaitConnect(ctx); err != nil {
				s.Fatal("Failed to reconnect to DUT: ", err)
			}

			// If stuck on USB boot, syncing tast files to usb image will allow kernel service to restore kernel on disk.
			s.Log("Syncing TAST File from host")
			if err := h.SyncTastFilesToDUT(ctx); err != nil {
				s.Error("Syncing Tast files to DUT failed, but trying to restore anyway: ", err)
			}

			s.Log("Sync KERN-A/B backups from host to DUT")
			if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), map[string]string{
				kernAHostBackup.Name(): kernelBackup.KernA.BackupPath,
				kernBHostBackup.Name(): kernelBackup.KernB.BackupPath,
			}, linuxssh.PreserveSymlinks); err != nil {
				s.Fatal("Failed to get backup files to DUT from host")
			}

			if err := h.RequireKernelServiceClient(ctx); err != nil {
				s.Error("Failed to connect to kernel service: ", err)
			}

			s.Log("Restoring kernel from backup")
			if _, err := h.KernelServiceClient.RestoreKernel(ctx, kernelBackup); err != nil {
				s.Error("Failed to restore kernel from backup: ", err)
			}

			s.Log("Performing mode aware reboot to ensure restored boot from disk")
			if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
				s.Error("Failed to reboot: ", err)
			}
		}

		s.Log("Delete backup files from DUT")
		rmargs := []string{
			kernelBackup.KernA.BackupPath,
			kernelBackup.KernB.BackupPath,
		}
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", rmargs...).Output(ssh.DumpLogOnError); err != nil {
			testing.ContextLog(ctx, "Failed to delete backup files: ", err)
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

	var state firmware.CheckAndSetServoCharger
	state = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	s.Log("Performing mode aware reboot to ensure boot to copy A")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy A")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to prioritize copy KERN-A: ", err)
	}

	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Could not determine boot device type: ", err)
	}
	if bootedFromRemovableDevice {
		s.Fatal("DUT unexpectedly booted from usb before corrupting kernel")
	}

	diskPath, err := getRootdevNameByID(ctx, h)
	if err != nil {
		s.Fatal("Failed to get rootdev path from /dev/disk/by-id: ", err)
	}

	if err := h.DUT.Conn().CommandContext(ctx, "elogtool", "clear").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to clear eventlog: ", err)
	}

	s.Log("Corrupt KERN-A")
	if _, err := h.KernelServiceClient.SetKernelHeaderMagic(ctx, &pb.KernelHeaderMagicInfo{
		Name:  pb.PartitionName_KERNEL,
		Copy:  pb.PartitionCopy_A,
		Magic: pb.KernelHeaderMagic_CORRUPTD,
	}); err != nil {
		s.Fatal("Failed to corrupt KERN-A: ", err)
	}

	s.Log("Corrupt KERN-B")
	if _, err := h.KernelServiceClient.SetKernelHeaderMagic(ctx, &pb.KernelHeaderMagicInfo{
		Name:  pb.PartitionName_KERNEL,
		Copy:  pb.PartitionCopy_B,
		Magic: pb.KernelHeaderMagic_CORRUPTD,
	}); err != nil {
		s.Fatal("Failed to corrupt KERN-B: ", err)
	}

	needsUSBRestore := true
	defer func(ctx context.Context) {
		if needsUSBRestore {
			s.Log("Booting to recovery mode to restore kernel from USB")
			if err := h.BootToRecoveryMode(ctx, &state); err != nil {
				s.Fatal("Failed to boot to recovery mode: ", err)
			}
			s.Log("Restore KERN-A")
			if err := restoreKernelHeaderFromUSB(ctx, h, diskPath, "2"); err != nil {
				s.Fatal("Failed to restore KERN-A: ", err)
			}
			s.Log("Restore KERN-B")
			if err := restoreKernelHeaderFromUSB(ctx, h, diskPath, "4"); err != nil {
				s.Fatal("Failed to restore KERN-B: ", err)
			}

			s.Log("Performing mode aware reboot to boot to original bootmode")
			if err := ms.RebootToMode(ctx, pv.BootMode, firmware.AllowGBBForce); err != nil {
				s.Fatal("Failed to reboot: ", err)
			}

			if !state.IsServoChargerConnected {
				if err := h.SetDUTPower(ctx, true); err != nil {
					s.Fatal("Failed to connect charger: ", err)
				}
				state.IsServoChargerConnected = true
				waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
				defer cancelWaitConnect()
				if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
					s.Fatal("Failed to reconnect to the DUT: ", err)
				}
			}

			s.Log("Syncing TAST File from host")
			if err := h.SyncTastFilesToDUT(ctx); err != nil {
				s.Fatal("Syncing Tast files to DUT failed, but trying to restore anyway: ", err)
			}
		}
	}(cleanupContext)

	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Setting usb mux state to off: ", err)
	}

	s.Log("Rebooting the DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		s.Fatal("Failed to run reboot command: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable, reboot cmd failed: ", err)
	}
	s.Log("Waiting for DUT to reach the firmware screen")
	if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
		s.Fatal("Failed to get to firmware screen: ", err)
	}
	s.Log("Checking if DUT stays at the Broken Screen")
	brokenToDevWaitConnectCtx, cancelWaitConnectBrokenToDev := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnectBrokenToDev()

	err = h.WaitConnect(brokenToDevWaitConnectCtx, firmware.ResetEthernetDongle)
	switch err.(type) {
	case nil:
		s.Fatal("DUT woke up unexpectedly")
	default:
		if !errors.As(err, &context.DeadlineExceeded) {
			s.Fatal("Unexpected error occurred: ", err)
		}
	}

	s.Log("Booting to recovery mode")
	if err := h.BootToRecoveryMode(ctx, &state); err != nil {
		s.Fatal("Failed to boot to recovery mode: ", err)
	}

	hasRecRes, err := h.Reporter.ContainsRecoveryReason(ctx, []reporters.RecoveryReason{
		reporters.RecoveryReasonDeprecatedRWNoDisk,
		reporters.RecoveryReasonRWInvalidOS,
		reporters.RecoveryReasonRWNoKernel,
	})
	if err != nil {
		s.Error("Failed to find recovery reason in crossystem params: ", err)
	} else if !hasRecRes {
		s.Error("Didn't find expected recovery reason in crossystem params")
	}

	s.Log("Restore KERN-A")
	if err := restoreKernelHeaderFromUSB(ctx, h, diskPath, "2"); err != nil {
		s.Error("Failed to restore KERN-A: ", err)
	}

	s.Log("Restore KERN-B")
	if err := restoreKernelHeaderFromUSB(ctx, h, diskPath, "4"); err != nil {
		s.Error("Failed to restore KERN-B: ", err)
	}

	if err := h.RebootWithVT2Command(ctx, pv.BootMode); err != nil {
		s.Fatal("Failed to reboot back to original boot mode: ", err)
	}

	bootedFromRemovableDevice, err = h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Could not determine boot device type: ", err)
	}
	if bootedFromRemovableDevice {
		s.Fatal("DUT unexpectedly booted from usb")
	}
	needsUSBRestore = false

	if !state.IsServoChargerConnected {
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to connect charger: ", err)
		}
		state.IsServoChargerConnected = true
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Verify DUT in copy A")
	if _, err := h.KernelServiceClient.VerifyKernelCopy(ctx, &pb.Partition{
		Copy: pb.PartitionCopy_A,
	}); err != nil {
		s.Fatal("Failed to verify booot from KERN-A: ", err)
	}
	needsRestore = false
}

// getRootdevNameByID gets the name of disk by id to look for it at the end of the test to recovery kernel.
// We can't use the actual path from /dev/ as the numbers in the device name/path are inconsistent across boots.
// Example: /dev/mmcblk1 -> /dev/disk/by-id/mmc-CUTB42_0x0f9f49c9.
func getRootdevNameByID(ctx context.Context, h *firmware.Helper) (string, error) {
	rootdev, err := h.DUT.Conn().CommandContext(ctx, "rootdev", "-s", "-d").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get rootdev")
	}

	lsOut, err := h.DUT.Conn().CommandContext(ctx, "ls", "-o", "/dev/disk/by-id").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get 'ls -o /dev/disk/by-id' output")
	}
	// Output looks like:
	// lrwxrwxrwx. 1 root 13 Feb 13 15:25 mmc-CUTB42_0x0f9f49c9 -> ../../mmcblk1

	dev := strings.TrimPrefix(strings.TrimSpace(string(rootdev)), "/dev/")
	regex := fmt.Sprintf(`(\S+)\s+->\s+\.\./\.\./%s\s`, dev)
	match := regexp.MustCompile(regex).FindStringSubmatch(string(lsOut))
	if match == nil {
		return "", errors.Errorf("failed to find rootdev %q in /dev/disk/by-id output %q", dev, string(lsOut))
	}

	pathByID := fmt.Sprintf("/dev/disk/by-id/%s", match[1])
	testing.ContextLog(ctx, "Rootdev id: ", pathByID)
	return pathByID, nil
}

func restoreKernelHeaderFromUSB(ctx context.Context, h *firmware.Helper, diskPath, part string) error {
	usbdevRaw, err := h.DUT.Conn().CommandContext(ctx, "rootdev", "-s", "-d").Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to get rootdev")
	}
	usbdev := strings.TrimSpace(string(usbdevRaw))

	// Example: (/dev/disk/by-id/mmc-CUTB42_0x0f9f49c9, 2) -> /dev/disk/by-id/mmc-CUTB42_0x0f9f49c9-part2.
	diskdevWithPart := fmt.Sprintf("%s-part%s", diskPath, part)

	// Add part to usb dev, eg. /dev/sda -> /dev/sda2.
	usbdevWithPart := fmt.Sprintf("%s%s", usbdev, part)

	testing.ContextLogf(ctx, "Resetting disk kernel %q from %q", diskdevWithPart, usbdevWithPart)
	args := []string{
		fmt.Sprintf("if=%s", usbdevWithPart),
		fmt.Sprintf("of=%s", diskdevWithPart),
		"conv=notrunc,nocreat",
		"oflag=sync",
		"bs=8", "count=1", // Just write the first 8 bytes of the header magic.
	}
	cmd := h.DUT.Conn().CommandContext(ctx, "dd", args...)

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed resetting kernel")
	}

	return nil
}
