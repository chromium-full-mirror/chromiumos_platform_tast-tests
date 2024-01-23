// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
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

// cleanup cleans up backup files. This should not be called in the tests code if test is not backup manager owner.
func (b *FirmwareBackupManager) cleanup() {
	for key, val := range b.backupPaths {
		os.Remove(val)
		delete(b.backupPaths, key)
	}
}
