// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// FirmwareType describes firmware type.
type FirmwareType string

// Supported firmware types.
const (
	FirmwareAP FirmwareType = "AP"
)

// FirmwareBackupManager is base struct of firmware backup manager.
type FirmwareBackupManager struct {
	remoteBackupDir string
	backupPaths     map[FirmwareType]string
}

// NewFirmwareBackupManager creates new instance of FirmwareBackupManager.
// remoteBackupDir is path to directory in which backups will be stored on the remote
func NewFirmwareBackupManager(remoteBackupDir string) *FirmwareBackupManager {
	return &FirmwareBackupManager{
		remoteBackupDir: remoteBackupDir,
		backupPaths:     make(map[FirmwareType]string),
	}
}

// backupFirmware performs backup of selected firmware and stores it on the remote. If backup is already available, then it won't be repeated unless Cleanup() is called before.
// ctx is context for all operations. It has to be long enough to allow for full backup, otherwise unknown side effects will occur.
// dut is instance of dut.DUT. If tan be nil if SSH connection is not available or SSH backup is not wanted.
// pxy is instance of servo.Proxy to use for backup via Servo. Can be nil if backup via Servo is not possible or is not wanted.
// fwtypes is one or many firmware types to perform backup of.
// Returns nil on success or error on failure. Availability of backup of one of selected firmwares on case of more than one is not guaranteed on error.
func (b *FirmwareBackupManager) backupFirmware(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy, fwTypes ...FirmwareType) error {
	var err error
	for _, fw := range fwTypes {
		switch fw {
		case FirmwareAP:
			err = b.backupAP(ctx, dut, pxy)
		default:
			testing.ContextLog(ctx, "Unsupported firmware type: ", fw)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *FirmwareBackupManager) backupAPviaSSH(ctx context.Context, dut *dut.DUT) (p string, retErr error) {
	futilityInstance, err := futility.NewLocalBuilder(dut).Build()
	if err != nil {
		return "", errors.Wrap(err, "failed to create futility instance")
	}

	// Create temporary file to store backup image.
	out, err := dut.Conn().CommandContext(ctx, "mktemp", "-t", "bios.bak.XXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to create temporary file on DUT")
	}
	tmpFileLocal := strings.TrimSpace(string(out))
	defer func() {
		dut.Conn().CommandContext(ctx, "rm", tmpFileLocal).Run()
	}()

	readOpts := futility.NewReadAPOptions(tmpFileLocal)
	if log, err := futilityInstance.ReadAP(ctx, readOpts); err != nil {
		testing.ContextLog(ctx, "Futility log: ", string(log))
		return "", errors.Wrap(err, "failed to read AP firmware")
	}

	tmpFileRemote := filepath.Join(b.remoteBackupDir, filepath.Base(tmpFileLocal))

	// Store backup on the local (test runner) machine. DUT's storage is not guaranteed to survive tests.
	if err := linuxssh.GetFile(ctx, dut.Conn(), tmpFileLocal, tmpFileRemote, linuxssh.PreserveSymlinks); err != nil {
		return "", errors.Wrap(err, "failed to get AP firmware backup from DUT")
	}

	return tmpFileRemote, nil
}

func (b *FirmwareBackupManager) backupAPviaServo(ctx context.Context, pxy *servo.Proxy) (p string, retErr error) {
	futilityInstance, err := futility.NewRemoteBuilder(pxy).Build()
	if err != nil {
		return "", errors.Wrap(err, "failed to create futility instance")
	}

	tmpFile, err := os.CreateTemp(b.remoteBackupDir, "bios.bak.*")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temporary file")
	}
	tmpFile.Close()

	readOpts := futility.NewReadAPOptions(tmpFile.Name())
	if log, err := futilityInstance.ReadAP(ctx, readOpts); err != nil {
		testing.ContextLog(ctx, "Futility log: ", string(log))
		return "", errors.Wrap(err, "failed to read AP firmware")
	}

	return tmpFile.Name(), nil
}

func (b *FirmwareBackupManager) backupAP(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy) (retErr error) {
	// Backup via SSH
	if dut != nil {
		p, err := b.backupAPviaSSH(ctx, dut)
		if err == nil {
			b.backupPaths[FirmwareAP] = p
			return
		}
		retErr = errors.Wrap(err, "failed to backup AP firmware via SSH")
		testing.ContextLog(ctx, "Failed to backup AP firmware via SSH: ", retErr)
	}

	// Backup via Servo
	if pxy != nil {
		p, err := b.backupAPviaServo(ctx, pxy)
		if err == nil {
			b.backupPaths[FirmwareAP] = p
			return
		}

		err = errors.Wrap(err, "failed to backup AP firmware via Servo")
		if retErr != nil {
			return errors.Join(retErr, err)
		}
		return err
	}

	return errors.New("failed to backup AP, no DUT nor ServoProxy provided")
}

// IsBackupAvailable checks if backup for firmware type is available.
// fwType can be only one of firmware type values and not the combination of them.
func (b *FirmwareBackupManager) IsBackupAvailable(fwType FirmwareType) bool {
	if val, ok := b.backupPaths[fwType]; ok {
		if _, err := os.Stat(val); err != nil {
			return false
		}
		return true
	}
	return false
}

// CopyBackupFile copies backups of selected firmware type to the destination.
// fwType can be only one of firmware type values and not the combination of them.
// dst is the destination file path on the remote.
// Returns error if backup is not available or on copying failure.
func (b *FirmwareBackupManager) CopyBackupFile(fwType FirmwareType, dst string) error {
	if !b.IsBackupAvailable(fwType) {
		return errors.Errorf("backup not available for firmware type: %v", fwType)
	}
	p := b.backupPaths[fwType]

	if err := fsutil.CopyFile(p, dst); err != nil {
		return errors.Wrapf(err, "failed to copy backup file from %q to %q", p, dst)
	}
	return os.Chmod(dst, 0666)
}

// CopyBackupToDut copies backups of selected firmware type to the destination on DUT.
// fwType can be only one of firmware type values and not the combination of them.
// dst is the destination file path on the DUT.
// Returns error if backup is not available or on copying failure.
func (b *FirmwareBackupManager) CopyBackupToDut(ctx context.Context, dut *dut.DUT, fwType FirmwareType, dst string) error {
	if !b.IsBackupAvailable(fwType) {
		return errors.Errorf("backup not available for firmware type: %v", fwType)
	}

	p := b.backupPaths[fwType]
	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), map[string]string{p: dst}, linuxssh.PreserveSymlinks); err != nil {
		return errors.Wrapf(err, "failed to copy backup file to DUT from %q to %q", p, dst)
	}
	return nil
}

// CopyBackupToServoProxy copies backups of selected firmware type to the destination on ServoProxy.
// fwType can be only one of firmware type values and not the combination of them.
// dst is the destination file path on the DUT.
// Returns error if backup is not available or on copying failure.
func (b *FirmwareBackupManager) CopyBackupToServoProxy(ctx context.Context, pxy *servo.Proxy, fwType FirmwareType, dst string) error {
	if !b.IsBackupAvailable(fwType) {
		return errors.Errorf("backup not available for firmware type: %v", fwType)
	}

	p := b.backupPaths[fwType]
	if err := pxy.PutFiles(ctx, false, map[string]string{p: dst}); err != nil {
		return errors.Wrapf(err, "failed to copy backup file to ServoProxy from %q to %q", p, dst)
	}
	return nil
}

// RestoreAPRWViaServo flashes the AP RW firmware sections from the backup back to the DUT
// over servo (assuming write protect is enabled), powers the DUT on if it stays in G3
// (b/314059450), and waits for SSH.
func (b *FirmwareBackupManager) RestoreAPRWViaServo(ctx context.Context, h *firmware.Helper) (retErr error) {
	if b == nil || !b.IsBackupAvailable(FirmwareAP) {
		return errors.Errorf("backup not available for firmware type: %v", FirmwareAP)
	}
	if h == nil {
		return errors.New("firmware helper is nil")
	}
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to servod")
	}
	testing.ContextLog(ctx, "Restoring AP RW firmware via servo")
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	out, err := h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
	if err != nil {
		return errors.Wrap(err, "failed to create servo temp dir")
	}
	servoTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		if err := h.ServoProxy.RunCommand(cleanupCtx, false, "rm", "-rf", servoTempDir); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "failed deleting servo temp dir"))
		}
	}()

	backupOnServoProxy := filepath.Join(servoTempDir, "bios_backup.bin")
	if err := b.CopyBackupToServoProxy(ctx, h.ServoProxy, FirmwareAP, backupOnServoProxy); err != nil {
		return errors.Wrap(err, "failed to copy backup to ServoProxy")
	}

	// futility doesn't know that you can't flash AP over C2D2, so switch the active controller to CCD.
	if hasC2D2, err := h.Servo.HasC2D2(ctx); err != nil {
		return errors.Wrap(err, "failed check c2d2")
	} else if hasC2D2 {
		if err := h.Servo.RequireCCD(ctx); err != nil {
			return errors.Wrap(err, "failed enable CCD")
		}
	}
	if out, err := h.ServoProxy.OutputCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
		"--mode=recovery", "--wp=1", "--host_only", "-i", backupOnServoProxy); err != nil {
		return errors.Wrapf(err, "failed restoring firmware via servo: %s", string(out))
	}
	// In b/314059450 it was discovered that some devices don't come back on after futility update.
	if hasEC, err := h.Servo.HasControl(ctx, string(servo.ECSystemPowerState)); err != nil {
		testing.ContextLog(ctx, "Error checking for chrome ec: ", err)
	} else if hasEC {
		testing.ContextLog(ctx, "Waiting for DUT to power on")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S0"); err != nil {
			return errors.Wrap(err, "failed to get power state after restoring firmware")
		}
		powerState, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get power state")
		}
		if powerState == "G3" {
			testing.ContextLog(ctx, "DUT is in G3, pressing power key to wake")
			press := servo.DurPress
			if h.Config != nil && h.Config.HoldPwrButtonPowerOn > 0 {
				press = servo.Dur(h.Config.HoldPwrButtonPowerOn)
			}
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, press); err != nil {
				return errors.Wrap(err, "failed to press power")
			}
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
				return errors.Wrap(err, "failed to wait for S0 after power button")
			}
		}
	}
	waitTimeout := 2 * time.Minute
	if h.Config != nil && h.Config.DelayRebootToPing > waitTimeout {
		waitTimeout = h.Config.DelayRebootToPing
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, waitTimeout)
	defer cancelWait()
	if err := h.WaitConnect(waitCtx, firmware.ResetEthernetDongle, firmware.SkipPDRoleSnk); err != nil {
		return errors.Wrap(err, "failed to WaitConnect after reset")
	}
	return nil
}

// cleanup cleans up backup files. This should not be called in the tests code if test is not backup manager owner.
func (b *FirmwareBackupManager) cleanup() {
	for key, val := range b.backupPaths {
		os.Remove(val)
		delete(b.backupPaths, key)
	}
}
