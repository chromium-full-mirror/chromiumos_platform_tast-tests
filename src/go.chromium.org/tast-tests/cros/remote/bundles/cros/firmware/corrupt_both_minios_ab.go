// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CorruptBothMiniOSAB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that DUT does not boot from corrupted MINIOS-A and MINIOS-B",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_usb", "firmware_bios", "firmware_level2", "firmware_ro"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01"},
		HardwareDeps: hwdep.D(hwdep.MiniOS()),
		ServiceDeps:  []string{"tast.cros.firmware.KernelService"},
		Fixture:      fixture.NormalMode,
		Timeout:      120 * time.Minute,
	})
}

func CorruptBothMiniOSAB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	var state firmware.CheckAndSetServoCharger
	state = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}
	backupMiniOS, err := h.KernelServiceClient.BackupKernel(ctx, &pb.KernelBackup{BackupMiniOS: true})
	if err != nil {
		s.Fatal("Failed to back up MINIOS-A and MINIOS-B: ", err)
	}
	needRestore := false
	usbBoot := false

	defer func() {
		if needRestore {
			if !h.DUT.Connected(ctx) || usbBoot {
				s.Log("Rebooting the DUT with a cold reset")
				if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
					s.Error("Failed to cold reset the DUT: ", err)
				}
				waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
				defer cancelWaitConnect()

				if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
					s.Error("Failed to reconnect to the DUT: ", err)
				}
			}
			h.DisconnectDUT(ctx)
			if err := h.RequireKernelServiceClient(ctx); err != nil {
				s.Error("Failed to connect to kernel service: ", err)
			}
			s.Log("Restoring MiniOS kernel")
			if _, err := h.KernelServiceClient.RestoreKernel(ctx, backupMiniOS); err != nil {
				s.Error("Failed to restore MINIOS-A and MINIOS-B: ", err)
			}
		}
		s.Log("Deleting backup files from DUT")
		rmargs := []string{
			backupMiniOS.MiniOSA.BackupPath,
			backupMiniOS.MiniOSB.BackupPath,
		}
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", rmargs...).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete backup files: ", err)
		}
		s.Log("Performing a mode aware reboot to ensure restored kernel takes effect")
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
			s.Error("Failed to reboot: ", err)
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
	}()

	s.Log("Corrupting MINIOS-A and MINIOS-B")
	for _, value := range []pb.PartitionCopy{pb.PartitionCopy_A, pb.PartitionCopy_B} {
		if _, err := h.KernelServiceClient.SetKernelHeaderMagic(ctx, &pb.KernelHeaderMagicInfo{
			Name:  pb.PartitionName_MINIOS,
			Copy:  value,
			Magic: pb.KernelHeaderMagic_CORRUPTD,
		}); err != nil {
			s.Fatal("Failed to corrupt: ", err)
		}
	}
	needRestore = true

	s.Log("Booting the DUT from corrupted MiniOS")
	if err := h.LaunchMiniOS(ctx, true, false); err != nil {
		s.Fatal("Failed to launch MiniOS: ", err)
	}
	if state.RemoveServoChargerRequired {
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// booting the DUT from USB to prevent USB disconnected issues.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}

	s.Log("Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		s.Logf("Failed to set pd data role to DFP: %.400s", err)
	}
	s.Log("Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to insert a valid USB to the DUT: ", err)
	}
	s.Log("Checking if DUT boots from USB")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
		s.Fatal("Failed to boot from USB: ", err)
	}
	usbBoot = true

	expectedBootMode, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeRecovery)
	if err != nil {
		s.Fatal("Failed to check dut boot mode: ", err)
	}
	if !expectedBootMode {
		s.Fatalf("Failed to boot from expected boot mode, expected mode: %s", fwCommon.BootModeRecovery)
	}

	out, err := h.Reporter.GetCBMEMLogs(ctx)
	if err != nil {
		s.Fatal("Failed to run cbmem command: ", err)
	}
	s.Log("Checking if DUT failed to boot from corrupted MiniOS")
	if err := h.ScanWithoutExpectedSequenceInSource(ctx, out, []string{"Failed to boot from MiniOS"}); err != nil {
		saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
		if saveLogErr := os.WriteFile(saveLogPath, []byte(out), 0666); saveLogErr != nil {
			s.Fatalf("Failed to write firmware log: %v, got %v", saveLogErr, err)
		}
		s.Fatal("Failed to match firmware screen string: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	usbBoot = false

	if state.RemoveServoChargerRequired {
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

	h.DisconnectDUT(ctx)
	if err := h.RequireKernelServiceClient(ctx); err != nil {
		s.Fatal("Failed to connect to kernel service: ", err)
	}

	s.Log("Restoring MINIOS-A and MINIOS-B")
	for _, value := range []pb.PartitionCopy{pb.PartitionCopy_A, pb.PartitionCopy_B} {
		if _, err := h.KernelServiceClient.SetKernelHeaderMagic(ctx, &pb.KernelHeaderMagicInfo{
			Name:  pb.PartitionName_MINIOS,
			Copy:  value,
			Magic: pb.KernelHeaderMagic_CHROMEOS,
		}); err != nil {
			s.Fatal("Failed to restore: ", err)
		}
	}
	needRestore = false
}
