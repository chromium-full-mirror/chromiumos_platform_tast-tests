// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWCorruptRecoveryCache,
		Desc: "Corrupt recovery cache and then check it's rebuilt",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Timeout:      2 * time.Hour,
		Vars:         []string{"firmware.skipFlashUSB"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"has_recovery_mrc_cache"},
		Params: []testing.Param{
			{
				Name:      "normal",
				ExtraAttr: []string{"firmware_meets_kpi", "firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
				Fixture:   fixture.NormalMode,
				Val:       common.BootModeNormal,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevMode,
				Val:     common.BootModeDev,
			},
		},
	})
}

// FWCorruptRecoveryCache checks for RECOVERY_MRC_CACHE, voids it
// and then goes back to recovery mode to see if it's rebuilt again.
func FWCorruptRecoveryCache(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	bootMode := s.Param().(common.BootMode)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Failed to require BiosServiceClient: ", err)
	}

	s.Log("Verifying RECOVERY_MRC_CACHE section exists")
	err := h.DUT.Conn().CommandContext(ctx, "futility", "read", "-r", "RECOVERY_MRC_CACHE", "/dev/null").Run()
	if err != nil {
		s.Fatal("Failed to run cmd on DUT: ", err)
	} else if errCode, ok := testexec.ExitCode(err); !ok || errCode != 0 {
		s.Fatal("Failed to find RECOVERY_MRC_CACHE section: ", err)
	}

	s.Log("Setup USB Key")
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

	mrcHostBackup, err := os.CreateTemp("", "mrcHostBackup")
	if err != nil {
		s.Fatal("Failed to create temporary dir for Recovery MRC Cache backup")
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		mrcHostBackup.Close()
		if err := os.Remove(mrcHostBackup.Name()); err != nil {
			s.Log("Failed to delete MRC cache back up dir from host")
		}
	}(cleanupContext)

	s.Log("Backing up current RECOVERY_MRC_CACHE section for safety")
	mrcPath, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Programmer: pb.Programmer_BIOSProgrammer,
		Section:    pb.ImageSection_RECOVERYMRCCACHEImageSection,
	})
	if err != nil {
		s.Fatal("Failed to backup current RECOVERY_MRC_CACHE region: ", err)
	}
	s.Log("RECOVERY_MRC_CACHE region backup is stored at: ", mrcPath.Path)

	defer func(ctx context.Context) {
		s.Log("Reconnecting to BiosService on DUT")
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to reconnect to BiosServiceClient on DUT: ", err)
		}

		s.Log("Restoring RECOVERY_MRC_CACHE image")
		// DANGER THIS WILL CORRUPT YOUR FLASH IF THE SECTION DOESN'T PERFECTLY ALIGN WITH THE FLASH BLOCK SIZE
		// DO NOT USE THIS FUNCTION
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, mrcPath); err != nil {
			s.Error("Failed to restore MRC_RECOVERY_CACHE image: ", err)
		}

		s.Log("Removing RECOVERY_MRC_CACHE image backups from DUT")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", mrcPath.Path).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete RECOVERY_MRC_CACHE image from DUT: ", err)
		}
	}(cleanupContext)

	s.Log("Copying kernel back up to host")
	if err := linuxssh.GetFile(ctx, h.DUT.Conn(), mrcPath.Path, mrcHostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy mrc backup to the host")
	}
	defer func(ctx context.Context) {
		s.Log("Sync MRC backup from host to DUT")
		if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{
			mrcHostBackup.Name(): mrcPath.Path,
		}, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to get backup files to DUT from host")
		}
	}(cleanupContext)

	s.Log("Corrupting RECOVERY_MRC_CACHE section")
	// DANGER THIS WILL CORRUPT YOUR FLASH IF THE SECTION DOESN'T PERFECTLY ALIGN WITH THE FLASH BLOCK SIZE
	// DO NOT USE THIS FUNCTION
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx,
		&pb.FWSectionInfo{
			Section:    pb.ImageSection_RECOVERYMRCCACHEImageSection,
			Programmer: pb.Programmer_BIOSProgrammer,
		}); err != nil {
		s.Fatal("Failed to corrupt RECOVERY_MRC_CACHE section: ", err)
	}

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	defer func(ctx context.Context) {
		s.Log("Rebooting out of recovery")
		if h.DUT.Connected(ctx) {
			// The power_state:reset command might cause an error (ec/cr50/servo: no data was sent from pty or unresponsive).
			// To prevent this issue, send the 'reboot' command in VT2.
			if err := h.RebootWithSSHCommand(ctx, bootMode); err != nil {
				s.Fatal("Failed to reboot with VT2 command: ", err)
			}
		} else {
			// Something went wrong, and the DUT is disconnected. Use the power_state:reset command instead.
			if err := h.CloseRPCConnection(ctx); err != nil {
				s.Fatal("Failed to close rpc connection: ", err)
			}
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatal("Failed to cold reset the DUT: ", err)
			}
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx); err != nil {
				s.Fatal("Failed to reconnect to DUT: ", err)
			}
		}
		s.Log("Checking that DUT has booted from internal disk")
		bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
		if err != nil {
			s.Fatal("Could not determine boot device type: ", err)
		}
		if bootedFromRemovableDevice {
			s.Fatalf("DUT did not boot from the internal device: got %v, want false", bootedFromRemovableDevice)
		}

		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
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
	}(cleanupContext)

	s.Log("Rebooting into recovery mode to check if RECOVERY_MRC_CACHE needs update")
	if err := h.BootToRecoveryMode(ctx, &state, false); err != nil {
		s.Fatal("Failed to boot to recovery mode: ", err)
	}

	if isExpected, err := h.Reporter.ContainsRecoveryReason(ctx, []reporters.RecoveryReason{reporters.RecoveryReasonROManual}); err != nil {
		s.Fatal("Failed to get the recovery reason")
	} else if !isExpected {
		s.Fatal("Failed to get expected recovery reason")
	}

	s.Log("Checking if recovery MRC cache has been rebuilt")
	const cbmemCheckCommand = `cbmem -1 | grep -e MRC -e APOB`
	out, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", cbmemCheckCommand).Output()
	if err != nil {
		s.Fatalf("Failed to grep for MRC from cbmem, got output %v with error: %v", string(out), err)
	}
	outstr := string(out)
	s.Log("Got output from cbmem: ", outstr)
	if !strings.Contains(outstr, "MRC: cache data 'RECOVERY_MRC_CACHE' needs update.") &&
		!strings.Contains(outstr, "APOB RAM hash differs from flash") {
		s.Fatal("Output from cbmem did not contain expected message: ", err)
	}
}
