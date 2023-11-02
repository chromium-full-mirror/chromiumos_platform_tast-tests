// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils contains functionality shared by tests that
// exercise firmware.
package utils

import (
	"context"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	// CCDPassword is a temporary Cr50 password to be set for testing purposes
	CCDPassword = "robot"
	// CCDWrongPassword is a constant with wrong Cr50 password that is tried to be set during the tests
	CCDWrongPassword = "wrongpass"
	// CCDClearPasswordPrefix is a prefix that has to be used before password to clear it with gsctool
	CCDClearPasswordPrefix = "clear:"
	// WaitAfterCCDSettingChange is a period of time to wait after changing a CCD setting because of rate limit
	WaitAfterCCDSettingChange = 3 * time.Second
)

// GSCBehavior contains behaviors of gsctool that can be passed to VerifyGsctoolCommand() function
type GSCBehavior string

// gsctool behaviors list
const (
	OpenGSC          GSCBehavior = "open"
	LockGSC          GSCBehavior = "lock"
	UnlockGSC        GSCBehavior = "unlock"
	SetGSCPassword   GSCBehavior = "setPassword"
	ClearGSCPassword GSCBehavior = "clearPassword"
)

// List of gsctool command options that are supported by VerifyGsctoolCommand() function
const (
	GsctoolOptOpen             = "-o"
	GsctoolOptLock             = "-k"
	GsctoolOptUnlock           = "-U"
	GsctoolOptSetClearPassword = "-P"
)

// List of CCD states
const (
	CCDOpened   = "Opened"
	CCDLocked   = "Locked"
	CCDUnlocked = "Unlocked"
)

// List of CCD password states
const (
	CCDPasswordSet  = "set"
	CCDPasswordNone = "none"
)

// ChangeFWVariant checks if current FW variant (A/B) is equal to the fwVar, if not it switches to the fwVar
func ChangeFWVariant(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, fwVar fwCommon.RWSection) error {
	testing.ContextLogf(ctx, "Check the firmware version, looking for %q", fwVar)
	if isFWVerCorrect, err := h.Reporter.CheckFWVersion(ctx, string(fwVar)); err != nil {
		return errors.Wrap(err, "failed to check a firmware version")
	} else if !isFWVerCorrect {
		testing.ContextLogf(ctx, "Set FW tries to %q", fwVar)
		if err := firmware.SetFWTries(ctx, h.DUT, fwVar, 0); err != nil {
			return errors.Wrapf(err, "failed to set FW tries to %q", fwVar)
		}

		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			return errors.Wrap(err, "failed to perform mode aware reboot")
		}

		testing.ContextLog(ctx, "Check the firmware version after reboot")
		if isFWVerCorrect, err := h.Reporter.CheckFWVersion(ctx, string(fwVar)); err != nil {
			return errors.Wrap(err, "failed to check a firmware version")
		} else if !isFWVerCorrect {
			return errors.New("failed to boot into the expected firmware version")
		}
	}
	return nil
}

// CheckRecReason checks if recovery reason occures in the expReason slice
func CheckRecReason(ctx context.Context, h *firmware.Helper, ms *firmware.ModeSwitcher, expReasons []reporters.RecoveryReason) error {
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		return errors.Wrap(err, "failed to set the USB Mux direction to the Host")
	}

	// Test element required if rebooting from recovery to anything
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "failed to remove watchdog for ccd")
	}

	if err := h.CloseRPCConnection(ctx); err != nil {
		return errors.Wrap(err, "failed to close RPC connection")
	}

	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		return errors.Wrap(err, "failed to warm reset DUT")
	}

	testing.ContextLog(ctx, "Sleeping 60 seconds to let DUT reboot and record recovery reason")
	// GoBigSleepLint: Give DUT time to reboot into recovery mode
	if err := testing.Sleep(ctx, 1*time.Minute); err != nil {
		return err
	}

	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to init servo")
	}

	// Recovery mode requires the DUT to boot the image on the USB.
	// Thus, the servo must show the USB to the DUT.
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to enable recovery mode")
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after booting to recovery mode")
	}

	if isRecovery, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeRecovery); err != nil {
		return errors.Wrap(err, "failed to check a boot mode")
	} else if !isRecovery {
		return errors.New("failed to boot into the recovery mode")
	}

	if containsRecReason, err := h.Reporter.ContainsRecoveryReason(ctx, expReasons); err != nil || !containsRecReason {
		return errors.Wrap(err, "failed to get expected recovery reason")
	}

	return nil
}

// CheckCrossystemWPSW returns an error if crossystem wpsw_cur value does not match expectedWPSW
func CheckCrossystemWPSW(ctx context.Context, h *firmware.Helper, expectedWPSW int) error {
	r := reporters.New(h.DUT)
	testing.ContextLog(ctx, "Check crossystem for write protect state param")
	strWPSW, err := r.CrossystemParam(ctx, reporters.CrossystemParamWpswCur)
	if err != nil {
		return errors.Wrapf(err, "failed to get crossystem %v value", reporters.CrossystemParamWpswCur)
	}
	currWPSW, err := strconv.Atoi(strWPSW)
	if err != nil {
		return errors.Wrap(err, "failed to convert crossystem wpsw value to integer value")
	}
	testing.ContextLogf(ctx, "Current write protect state: %v, Expected state: %v", currWPSW, expectedWPSW)
	if currWPSW != expectedWPSW {
		return errors.Errorf("expected WP state to %v, is actually %v", expectedWPSW, currWPSW)
	}
	return nil
}

// VerifyCr50Command runs a command in Cr50 console and checks CCD state afterwards.
func VerifyCr50Command(ctx context.Context, h *firmware.Helper, cmd, expectCCDState, expectCCDPasswdState string, expectReboot bool) error {
	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get boot id")
	}
	err = h.Servo.RunCR50Command(ctx, cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to execute %q", cmd)
	}
	if expectReboot {
		if err := WaitForDUTReboot(ctx, h, bootID); err != nil {
			return errors.Wrap(err, "reboot failed")
		}
	} else {
		if !h.DUT.Connected(ctx) {
			if err := WaitForDUTReboot(ctx, h, bootID); err != nil {
				return errors.Wrap(err, "unexpected reboot failed")
			}
			return errors.New("DUT rebooted unexpectedly")
		}
		// GoBigSleepLint: This sleep is needed to allow ccd settings to get set.
		if err := testing.Sleep(ctx, WaitAfterCCDSettingChange); err != nil {
			return errors.Wrap(err, "failed to wait for ccd setting to change")
		}
	}
	if err = CheckExpectedCCDState(ctx, h, expectCCDState, expectCCDPasswdState); err != nil {
		return errors.Wrap(err, "checkExpectedCCDState() failed")
	}
	return nil
}

// VerifyGsctoolCommand runs a gsctool command in developer console and checks CCD state afterwards.
func VerifyGsctoolCommand(ctx context.Context, h *firmware.Helper, behavior GSCBehavior, expectCCDState, expectCCDPasswdState string, expectReboot, expectFail, useWrongPassword bool) error {
	options := map[GSCBehavior]string{
		OpenGSC:          GsctoolOptOpen,
		LockGSC:          GsctoolOptLock,
		UnlockGSC:        GsctoolOptUnlock,
		SetGSCPassword:   GsctoolOptSetClearPassword,
		ClearGSCPassword: GsctoolOptSetClearPassword,
	}
	passwordIsSet := false
	_, ccdPasswdState, err := GetCCDStatePasswd(ctx, h)
	if err != nil {
		return errors.Wrap(err, "GetCCDStatePasswd() failed")
	}
	if ccdPasswdState != CCDPasswordNone {
		passwordIsSet = true
	}

	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get boot id")
	}
	if _, ok := options[behavior]; !ok {
		return errors.Errorf("unknown behavior %q, supported: %+q", behavior, reflect.ValueOf(options).MapKeys())
	}
	cmd := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", options[behavior])
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return errors.Wrap(err, "StdinPipe() failed")
	}
	defer cmd.Wait()
	testing.ContextLog(ctx, "Starting gsctool")
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "Start() failed")
	}

	ccdPasswd := CCDPassword
	if useWrongPassword {
		ccdPasswd = CCDWrongPassword
	}
	if behavior == ClearGSCPassword {
		ccdPasswd = CCDClearPasswordPrefix + ccdPasswd
	}

	if behavior == SetGSCPassword || behavior == ClearGSCPassword || passwordIsSet {
		testing.ContextLog(ctx, "Entering password")
		// Enter password twice
		if _, err := io.WriteString(stdin, ccdPasswd+"\n"+ccdPasswd+"\n"); err != nil {
			return errors.Wrap(err, "WriteString() failed")
		}
	}
	err = cmd.Wait()
	if err != nil && !expectFail {
		return errors.Wrap(err, "gsctool failed")
	}
	if err == nil && expectFail {
		return errors.Wrap(err, "gsctool did not fail as expected")
	}
	if expectReboot {
		if err := WaitForDUTReboot(ctx, h, bootID); err != nil {
			return errors.Wrap(err, "reboot failed")
		}
	} else {
		if !h.DUT.Connected(ctx) {
			if err := WaitForDUTReboot(ctx, h, bootID); err != nil {
				return errors.Wrap(err, "unexpected reboot failed")
			}
			return errors.New("DUT rebooted unexpectedly")
		}
		// GoBigSleepLint: This sleep is needed to allow ccd settings to get set.
		if err := testing.Sleep(ctx, WaitAfterCCDSettingChange); err != nil {
			return errors.Wrap(err, "failed to wait for ccd setting to change")
		}
	}

	if err = CheckExpectedCCDState(ctx, h, expectCCDState, expectCCDPasswdState); err != nil {
		return errors.Wrap(err, "checkExpectedCCDState() failed")
	}
	return nil
}

// WaitForDUTReboot checks if the DUT has been rebooted as expected.
func WaitForDUTReboot(ctx context.Context, h *firmware.Helper, bootID string) error {
	testing.ContextLog(ctx, "Waiting for connection to DUT")
	reconnectTimeout := 3 * time.Minute
	connectCtx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}
	newBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get new boot id")
	}
	if newBootID == bootID {
		return errors.Wrap(err, "unexpectedly got same boot id over reboot")
	}
	return nil
}

// GetCCDStatePasswd gets values of State and Password fields from ccd command.
func GetCCDStatePasswd(ctx context.Context, h *firmware.Helper) (string, string, error) {
	out, err := h.Servo.RunCR50CommandGetOutput(ctx, "ccd", []string{
		`State:\s*(\S+)\s*\n`,
		`Password:\s*(\S+)\s*\n`})
	if err != nil {
		return "", "", errors.Wrap(err, "function RunCR50CommandGetOutput() returned an error")
	}
	return out[0][1], out[1][1], nil
}

// CheckExpectedCCDState verifies if values of State and Password fields from ccd command are the expected ones.
func CheckExpectedCCDState(ctx context.Context, h *firmware.Helper, expectCCDState, expectCCDPasswdState string) error {
	ccdState, ccdPasswdState, err := GetCCDStatePasswd(ctx, h)
	if err != nil {
		return errors.Wrap(err, "GetCCDStatePasswd() failed")
	}
	if ccdState != expectCCDState {
		err = errors.Errorf("expected CCD state: %q, got %q", expectCCDState, ccdState)
		if ccdPasswdState != expectCCDPasswdState {
			err = errors.Errorf("%s; expected CCD password state: %q, got %q", err, expectCCDPasswdState, ccdPasswdState)
		}
	} else if ccdPasswdState != expectCCDPasswdState {
		err = errors.Errorf("expected CCD password state: %q, got %q", expectCCDPasswdState, ccdPasswdState)
	}
	if err != nil {
		return errors.Wrap(err, "unexpected CCD state")
	}
	testing.ContextLog(ctx, "CCD state as expected")
	return nil
}

// Cr50Cleanup ensures that the test leaves CCD opened and with no password set.
func Cr50Cleanup(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Cleanup after test")

	ccdState, _, err := GetCCDStatePasswd(ctx, h)
	if err != nil {
		return errors.Wrap(err, "GetCCDStatePasswd() failed")
	}
	if ccdState != CCDOpened {
		testing.ContextLog(ctx, "Running ccd testlab open from Cr50 console")
		if err := h.Servo.RunCR50Command(ctx, "ccd testlab open"); err != nil {
			return errors.Wrap(err, `failed to execute "ccd testlab open"`)
		}
		// GoBigSleepLint: This sleep is needed to allow ccd settings to get set.
		if err := testing.Sleep(ctx, WaitAfterCCDSettingChange); err != nil {
			return errors.Wrap(err, "failed to wait for ccd setting to change")
		}
	}
	testing.ContextLog(ctx, "Reset CCD")
	if _, err = h.Servo.RunCR50CommandGetOutput(ctx, "ccd reset", []string{`Resetting\s+all\s+settings`}); err != nil {
		return errors.Wrap(err, "ccd reset failed")
	}
	return err
}

// BackupAndRestoreAPFirmwareAndWriteProtect makes a backup of AP firmware.
// Returns a context derived from ctx with time reserved for cleanup. Returns a
// closure for restoring the AP firmware and resetting hardware and software
// write protect to the off state. Returns a path to the firmware backup file on
// the DUT disk.
func BackupAndRestoreAPFirmwareAndWriteProtect(ctx context.Context, DUT *dut.DUT, servoCon *servo.Servo, flash *flashrom.Instance) (context.Context, func(s *testing.State), *string, error) {
	// /var/tmp is used, as /tmp will be cleared on reboot.
	// Reboot is often required as asurada cannot change write protect without a reboot.
	backupTmpFileStdout, err := DUT.Conn().CommandContext(ctx, "mktemp", "--tmpdir=/var/tmp", "tast.firmware.APFW.XXXXXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create a temp file")
	}
	backupTmpFile := strings.TrimSpace(string(backupTmpFileStdout))

	if out, err := flash.Read(ctx, backupTmpFile, nil); err != nil {
		testing.ContextLog(ctx, out)
		return nil, nil, nil, errors.Wrap(err, "failed to create a AP firmware backup")
	}

	// Reserve time for the backup to be restored if the test times out.
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	return ctx, func(s *testing.State) {
		s.Log("Disable hardware write protect")
		if err := servoCon.SetFWWPState(cleanupContext, servo.FWWPStateOff); err != nil {
			s.Error("Failed to disable hardware write protect: ", err)
		}

		s.Log("Disable software write protect")
		if out, err := flash.SoftwareWriteProtectDisable(cleanupContext); err != nil {
			s.Log(out)
			s.Error("Failed to disable software write protect: ", err)
		}

		s.Log("Restore AP firmware")
		if out, err := flash.Write(cleanupContext, backupTmpFile, false /* noVerifyAll */, true /* noVerify */, "" /* flashcontentsImage */, nil /* region */); err != nil {
			s.Log(out)
			s.Error("Failed to restore AP firmware backup: ", err)
		}

		DUT.Conn().CommandContext(cleanupContext, "rm", backupTmpFile).Output(ssh.DumpLogOnError)

		cancel()
	}, &backupTmpFile, nil
}

// EnableSoftwareSync backs up EC_RW and clears the DISABLE_EC_SOFTWARE_SYNC gbb flag if set and provides a restore function which cleans up side effects.
func EnableSoftwareSync(ctx context.Context, h *firmware.Helper, syncBackup bool) (func(context.Context, *testing.State), error) {
	if err := h.RequireBiosServiceClient(ctx); err != nil {
		return nil, errors.Wrap(err, "requiring BiosServiceClient")
	}

	testing.ContextLog(ctx, "Get intial GBB flags")
	old, err := fwCommon.GetGBBFlags(ctx, h.DUT)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial GetGBBFlags")
	}

	testing.ContextLog(ctx, "Backing up current EC_RW")
	backup, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    pb.ImageSection_ECRWImageSection,
		Programmer: pb.Programmer_ECProgrammer,
		Path:       "/usr/local/share/tast/",
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not backup EC firmware")
	}

	testing.ContextLog(ctx, "Check DISABLE_EC_SOFTWARE_SYNC GBB flag is not set, if it is, clear it")
	if fwCommon.GBBFlagsContains(old, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
		testing.ContextLog(ctx, "Clearing GBB flag DISABLE_EC_SOFTWARE_SYNC")
		req := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}

		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req); err != nil {
			return nil, errors.Wrap(err, "failed to clear gbb flag")
		}
	}

	restoreFunc := func(cleanupCtx context.Context, s *testing.State) {
		s.Log("Restoring EC_RW from backup")
		h.DisconnectDUT(cleanupCtx)
		if err := h.EnsureDUTBooted(cleanupCtx); err != nil {
			s.Fatal("Can't restore firmware, DUT is off: ", err)
		}

		if syncBackup {
			// Don't use s.Fatal here in case restore was used after fw backed up but
			// failed to save files locally so fw can still be cleaned up.
			s.Log("Syncing EC RW backup to DUT")
			if err := h.SyncTastFilesToDUT(cleanupCtx); err != nil {
				s.Error("Could not restore saved tast files: ", err)
			}
		}

		if err := h.RequireBiosServiceClient(cleanupCtx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}
		s.Log("Restoring EC firmware backup")
		if _, err := h.BiosServiceClient.RestoreImageSection(cleanupCtx, backup); err != nil {
			s.Fatal("Failed to restore EC firmware: ", err)
		}
		s.Log("Deleting temp file")
		if err := h.DUT.Conn().CommandContext(cleanupCtx, "rm", "-f", backup.Path).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete firmware backup: ", err)
		}

		// Reboot and check active copy after restore.
		ms, err := firmware.NewModeSwitcher(cleanupCtx, h)
		if err != nil {
			s.Fatal("Creating mode switcher: ", err)
		}
		if err := ms.ModeAwareReboot(cleanupCtx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
		s.Log("Checking ec_active_copy is RW or RW_B")
		activeCopy, err := h.Servo.GetString(cleanupCtx, "ec_active_copy")
		if err != nil {
			s.Fatal("EC active copy failed: ", err)
		}
		if !strings.HasPrefix(activeCopy, "RW") {
			s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
		}
	}

	if syncBackup {
		testing.ContextLog(ctx, "Copying EC RW back up so it can be restored after reset")
		if err := h.CopyTastFilesFromDUT(ctx); err != nil {
			return restoreFunc, errors.Wrap(err, "could not save tast files")
		}
	}

	return restoreFunc, nil
}
