// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const shellScript = `flashrom -p ec -w "$1"
echo "FLASHROM EXIT: $?"
set +x
ectool echash start rw
until ectool echash | grep 'done' ; do : ; done
echo -n "AFTER "
ectool echash | grep "hash:"
sync
`

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptBothFWSigABAndEC,
		Desc: "Corrupt both the firmware signature A/B and the EC, then verify the success of the recovery boot from the USB",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_bios", "firmware_stressed", "firmware_meets_kpi", "firmware_bios_ro", "firmware_ec_ro"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnPlatform("fizz", "kalista")),
		Timeout:      2 * time.Hour,
		Vars:         []string{"firmware.skipFlashUSB"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
			},
			{
				Name:    "dev",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
			},
		},
	})
}

func CorruptBothFWSigABAndEC(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
	backupManager := pv.BackupManager
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	s.Log("Setup USB Key")
	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
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

	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/usr/local/tmp", "-t", "shellscriptXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to create remote temp dir: ", err)
	}
	shellDir := strings.TrimSuffix(string(out), "\n")
	defer func(ctx context.Context) {
		if err := h.DUT.Conn().CommandContext(ctx, "rm", "-r", shellDir).Run(ssh.DumpLogOnError); err != nil {
			s.Error("Failed to delete shell dir: ", err)
		}
	}(ctx)
	if err := linuxssh.WriteFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/ecflash.sh", shellDir), []byte(shellScript), 0755); err != nil {
		s.Fatal("Failed to write flash script: ", err)
	}

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)
	defer func(ctx context.Context) {
		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Error("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
		}
	}(ctx)

	backupState, err := firmware.BackupECFirmware(ctx, h)
	if err != nil {
		s.Fatal("Failed to backup EC: ", err)
	}
	defer func(ctx context.Context) {
		if err := backupState.Close(ctx, h); err != nil {
			s.Error("Failed to cleanup EC backup: ", err)
		}
	}(ctx)

	s.Log("Get initial GBB flags")
	oldGBBFlags, err := fwCommon.GetGBBFlags(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed to get gbb flags: ", err)
	}
	defer func(ctx context.Context) {
		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, oldGBBFlags); err != nil {
			s.Error("Failed to set gbb flags: ", err)
		}
	}(ctx)

	s.Log("Check if the DISABLE_EC_SOFTWARE_SYNC GBB flag is set; if it is, clear it")
	if fwCommon.GBBFlagsContains(oldGBBFlags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
		backupState.ShouldRestoreFirmware = true
		s.Log("Clearing GBB flag DISABLE_EC_SOFTWARE_SYNC")
		req := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}

		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req); err != nil {
			s.Fatal("Failed to set gbb flag: ", err)
		}
	}

	s.Log("Checking preconditions")
	// TODO(b/194910957): Old test checks that fw-a section does not have preamble flag PREAMBLE_USE_RO_NORMAL. this really needed?

	// Reboot just in case the firmware version we backed up isn't the same one that software sync will restore.
	// Perform a cold reset to prevent "RO_AT_BOOT is not clear" errors.
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	if err := h.Servo.CheckECActiveCopyMatch(ctx, "RW"); err != nil {
		s.Fatal("Failed to verify EC active copy: ", err)
	}

	needsUSBRestore := true
	defer func(ctx context.Context) {
		s.Log("Restore firmware")
		if needsUSBRestore {
			s.Log("Booting to recovery mode to restore firmware signs")
			if err := h.BootToRecoveryMode(ctx, &state, false); err != nil {
				s.Error("Failed to boot to recovery mode: ", err)
			}
		}
		s.Log("Rollback the DUT with recovery mode using original bios binary file")
		out, err = h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Error("Failed creating remote temp dir: ", err)
		}
		dutTempDir := strings.TrimSuffix(string(out), "\n")
		apBackupOnDut := filepath.Join(dutTempDir, "bios.bin")

		if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, apBackupOnDut); err != nil {
			s.Error("Failed to copy AP firmware binary to DUT: ", err)
		}
		futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
		if err != nil {
			s.Error("Failed to create futility instance: ", err)
		}
		recoveryOpts := futility.
			NewUpdateOptions(apBackupOnDut).
			WithMode(futility.UpdateModeRecovery).
			WithWriteProtection(futility.WriteProtectionEnable).
			WithHostOnly(true).
			WithForce(true)

		if _, err := futilityInstance.Update(ctx, recoveryOpts); err != nil {
			s.Error("Failed to use futility update to restore the firmware: ", err)
		}
		// The power_state:reset command might cause an error (ec/cr50/servo: no data was sent from pty or unresponsive).
		// To prevent this issue, send the 'reboot' command in VT2.
		if err := rebootAfterFlash(ctx, h, pv.BootMode); err != nil {
			s.Error("Failed to reboot after flashing DUT: ", err)
		}
	}(ctx)

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Failed to require BiosServiceClient: ", err)
	}
	s.Log("Corrupt firmware A/B signatures")
	// DANGER THIS WILL CORRUPT YOUR FLASH IF THE SECTION DOESN'T PERFECTLY ALIGN WITH THE FLASH BLOCK SIZE
	// DO NOT USE THIS FUNCTION
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{Section: pb.ImageSection_FWSignAImageSection, Programmer: pb.Programmer_BIOSProgrammer}); err != nil {
		s.Fatal("Failed to corrupt Firmware A Sign (VBOOTA) section: ", err)
	}
	// DANGER THIS WILL CORRUPT YOUR FLASH IF THE SECTION DOESN'T PERFECTLY ALIGN WITH THE FLASH BLOCK SIZE
	// DO NOT USE THIS FUNCTION
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{Section: pb.ImageSection_FWSignBImageSection, Programmer: pb.Programmer_BIOSProgrammer}); err != nil {
		s.Fatal("Failed to corrupt Firmware B Sign (VBOOTB) section: ", err)
	}
	if err := h.Servo.CheckECActiveCopyMatch(ctx, "RW"); err != nil {
		s.Fatal("Failed to verify EC active copy: ", err)
	}

	s.Log("Corrupt the EC section: ", bios.RWFWIDImageSection)
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", fmt.Sprintf("%s/ec_backup.bin", backupState.RemoteTempDir()), fmt.Sprintf("%s:%s/fwid.good", bios.RWFWIDImageSection, backupState.RemoteTempDir())).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to extract fwid.good: ", err)
	}
	if err := linuxssh.WriteFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/fwid.bad", backupState.RemoteTempDir()), []byte("invalid_version"), 0644); err != nil {
		s.Fatal("Failed to write fwid.bad: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "truncate", "-c", "-r", fmt.Sprintf("%s/fwid.good", backupState.RemoteTempDir()), fmt.Sprintf("%s/fwid.bad", backupState.RemoteTempDir())).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to pad fwid.bad: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/ec_corrupt.bin", backupState.RemoteTempDir()), fmt.Sprintf("%s/ec_backup.bin", backupState.RemoteTempDir()), fmt.Sprintf("%s:%s/fwid.bad", bios.RWFWIDImageSection, backupState.RemoteTempDir())).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to write ec_corrupt.bin: ", err)
	}
	s.Log("Removing USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to remove the USB: ", err)
	}
	backupState.ShouldRestoreFirmware = true
	s.Log("Set FW tries to A")
	if err := firmware.SetFWTries(ctx, h.DUT, fwCommon.RWSectionA, 0); err != nil {
		s.Fatal("Failed to set FW tries to A: ", err)
	}

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Fatal("Failed to enable capture EC UART: ", err)
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}()
	}

	if err := flashCorruptEC(ctx, h, fmt.Sprintf("%s/ec_corrupt.bin", backupState.RemoteTempDir()), shellDir); err != nil {
		s.Fatal("Failed to corrupt ec: ", err)
	}
	if err := h.CloseRPCConnection(ctx); err != nil {
		s.Fatal("Failed to close RPC connection: ", err)
	}

	hasBrokenScreen := pv.BootMode != fwCommon.BootModeDev || !h.Config.NoBrokenScreenInDev
	for _, section := range []fwCommon.RWSection{
		fwCommon.RWSectionA,
		fwCommon.RWSectionB,
	} {
		if section == fwCommon.RWSectionB {
			s.Log("Set FW tries to B")
			if err := firmware.SetFWTries(ctx, h.DUT, section, 0); err != nil {
				s.Fatal("Failed to set FW tries to B: ", err)
			}
			s.Log("Rebooting the DUT")
			if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
				s.Fatal("Failed to run reboot command: ", err)
			}
			waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelWaitDisconnect()
			if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
				s.Fatal("Failed to wait for DUT to become unreachable, reboot failed: ", err)
			}
			s.Log("Removing USB")
			if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
				s.Fatal("Failed to remove the USB: ", err)
			}
		}

		s.Log("Waiting for DUT to reach the firmware screen")
		if h.HasAPFwState {
			if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.RecoveryBroken); err != nil {
				s.Fatal("Failed to detect firmware screen: ", err)
			}
		} else {
			if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreenRecMode); err != nil {
				s.Fatal("Failed to get to firmware screen: ", err)
			}
		}

		s.Log("Checking if EC active copy is RO")
		if err := h.Servo.CheckECActiveCopyMatch(ctx, "RO"); err != nil {
			s.Fatal("Failed to verify EC active copy: ", err)
		}
		if state.RemoveServoChargerRequired && state.IsServoChargerConnected {
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
			testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
		}
		s.Log("Inserting the USB to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to insert a valid USB to DUT: ", err)
		}
		if !h.HasAPFwState {
			if err := h.WaitDUTConnectDuringBootFromUSB(ctx, !hasBrokenScreen); err != nil {
				s.Fatalf("Failed to get expected behavior, expected stay in broken screen: %v: %v", hasBrokenScreen, err)
			}
		}
		if hasBrokenScreen {
			if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
				if err := h.SetDUTPower(ctx, true); err != nil {
					s.Fatal("Failed to connect charger: ", err)
				}
				state.IsServoChargerConnected = false
				// GoBigSleepLint: Wait for a while for connecting the charger.
				if err := testing.Sleep(ctx, 5*time.Second); err != nil {
					s.Fatal("Failed to sleep: ", err)
				}
			}
			s.Log("Booting the DUT from USB")
			if err := h.BootToRecoveryMode(ctx, &state, false); err != nil {
				s.Fatal("Failed to boot to the recovery mode: ", err)
			}
		}

		s.Log("Checking if crossystem recovery_reason is ", reporters.RecoveryReasonROInvalidRW)
		if isExpected, err := h.Reporter.ContainsRecoveryReason(ctx, []reporters.RecoveryReason{reporters.RecoveryReasonROInvalidRW}); err != nil {
			s.Fatal("Failed to check the recovery reason: ", err)
		} else if !isExpected {
			s.Fatal("Failed to get the expected recovery reason")
		}
	}
	needsUSBRestore = false
}

// flashCorruptEC flashes a corrupted firmware file to the EC. The corrupted file must already be present on the DUT.
// This function causes the DUT to remain on the broken screen.
// Returns an error if the EC hash does not change after flashing.
func flashCorruptEC(ctx context.Context, h *firmware.Helper, imagePath, shellPath string) (retErr error) {
	hashBefore, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "ectool echash | grep hash: | sed \"s/hash:\\s\\+//\"").Output()
	if err != nil {
		return errors.Wrap(err, "failed to get ec hash before flashing EC")
	}
	hashBefore = bytes.TrimSuffix(hashBefore, []byte{'\n'})
	if len(hashBefore) == 0 {
		return errors.Wrap(err, "received an empty EC hash")
	}
	testing.ContextLogf(ctx, "hashBefore: %q", hashBefore)

	defer func() {
		testing.ContextLog(ctx, "Rebooting the DUT with EC reboot command")
		if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "failed to ping EC console"))
		}
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "DUT failed to reach S0 after executing EC reboot command"))
		}
	}()
	if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", fmt.Sprintf("stdbuf -oL -eL nohup '%[1]s/ecflash.sh' '%[2]s' &>'%[1]s/ecflash.log' & exit", shellPath, imagePath)).Start(); err != nil {
		return errors.Wrap(err, "failed to run ec flash")
	}
	h.CloseRPCConnection(ctx)
	if err := verifyHashChange(ctx, h, shellPath, hashBefore); err != nil {
		return errors.Wrap(err, "failed to verify ec hash has changed")
	}
	return nil
}

// verifyHashChange first attempts polling via the ecflash.log.
// Some boards (e.g., Coral) may lose Ethernet connectivity during an EC update due to a USB bus reset (see e.g., b/188495004),
// which can prevent reading from ecflash.log. If polling fails, the 'hash' EC command is used as a fallback.
func verifyHashChange(ctx context.Context, h *firmware.Helper, shellPath string, hashBefore []byte) error {
	var flashromExitCodeRe = regexp.MustCompile(`FLASHROM EXIT: (-?\d+)`)
	var hashAfterRe = regexp.MustCompile(`AFTER hash:\s*(\S+)`)

	if ecLogErr := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := linuxssh.ReadFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/ecflash.log", shellPath))
		if err != nil {
			return errors.Wrap(err, "failed to read ecflash.log")
		}
		if m := flashromExitCodeRe.FindSubmatch(out); m == nil || string(m[1]) != "0" {
			return errors.New("flashrom failed")
		}
		m := hashAfterRe.FindSubmatch(out)
		if m == nil {
			return errors.Errorf("failed to get hash after flash: %s", string(out))
		}
		hashAfter := m[1]
		if bytes.Equal(hashAfter, hashBefore) {
			return errors.Errorf("the EC hash remained unchanged: %s", string(hashAfter))
		}
		testing.ContextLogf(ctx, "Corrupt hash: %s, Orignal hash: %s", string(hashAfter), string(hashBefore))
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); ecLogErr != nil {
		testing.ContextLogf(ctx, "Failed to verify hash from ecflash.log, got error: %s, try EC command: hash", ecLogErr.Error())
		if err := pollForECHashChange(ctx, h, hashBefore); err != nil {
			return errors.Join(err, ecLogErr)
		}
	}
	return nil
}

// pollForECHashChange polls to get EC hash by executing the EC command: hash.
func pollForECHashChange(ctx context.Context, h *firmware.Helper, hashBefore []byte) error {
	if retErr := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Servo.RunECCommandGetOutput(ctx, "hash", []string{`Digest:\s+(\S*)\s`})
		if err != nil {
			return errors.Wrap(err, "failed to get 'hash' output from ec console")
		}
		hashAfter := bytes.TrimSuffix([]byte(out[0][1]), []byte{'\n'})
		if len(hashBefore) != len(hashAfter) {
			return errors.Wrapf(err, "the length of the new EC hash is wrong: %s", string(hashAfter))
		}
		if bytes.Equal(hashAfter, hashBefore) {
			return errors.Wrapf(err, "the EC hash remained unchanged: %s", string(hashAfter))
		}
		testing.ContextLogf(ctx, "Corrupt hash: %s, Orignal hash: %s", string(hashAfter), string(hashBefore))
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); retErr != nil {
		return retErr
	}
	return nil
}

// rebootAfterFlash sends a reboot command via SSH to reboot the DUT.
func rebootAfterFlash(ctx context.Context, h *firmware.Helper, bootMode fwCommon.BootMode) error {
	if err := h.RebootWithSSHCommand(ctx, bootMode); err != nil {
		if errors.As(err, &context.DeadlineExceeded) {
			// It may take a longer time for DUT to reboot after restoring firmware.
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 10*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				return errors.Wrap(err, "failed to reconnect to the DUT")
			}
		} else {
			return errors.Wrap(err, "failed to reboot with VT2 command")
		}
	}
	return nil
}
