// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	remote_dut "go.chromium.org/tast-tests/cros/remote/dut"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const shellScript = `ectool echash start rw
until ectool echash | grep 'done' ; do : ; done
echo -n "BEFORE "
ectool echash | grep "hash:"
set -x
flashrom -p ec -w "$1"
echo "FLASHROM EXIT: $?"
set +x
ectool echash start rw
until ectool echash | grep 'done' ; do : ; done
echo -n "AFTER "
ectool echash | grep "hash:"
reboot
`

var flashromExitCodeRe = regexp.MustCompile(`FLASHROM EXIT: (-?\d+)`)
var hashBeforeRe = regexp.MustCompile(`BEFORE hash:\s*(\S+)`)
var hashAfterRe = regexp.MustCompile(`AFTER hash:\s*(\S+)`)

// BackupState handles cleanup of a firmware backup.
type BackupState struct {
	remoteTempDir         string
	dut                   *dut.DUT
	ShouldRestoreFirmware bool
	backupPath            string
}

// Close cleans up the BackupState. You must call this as a defer immediately after BackupECFirmware.
func (bs *BackupState) Close(ctx context.Context, h *Helper) (retErr error) {
	if bs.ShouldRestoreFirmware {
		retErr = bs.Restore(ctx, h)
	}
	err := bs.dut.Conn().CommandContext(ctx, "rm", "-rf", bs.remoteTempDir).Run(ssh.DumpLogOnError)
	if err != nil {
		retErr = errors.Join(retErr, errors.Wrap(err, "failed deleting remote temp dir"))
	}
	return
}

// RemoteTempDir returns the path to the temporary directory on the DUT.
func (bs *BackupState) RemoteTempDir() string {
	return bs.remoteTempDir
}

func waitForReboot(ctx context.Context, bootID string, h *Helper) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := h.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect")
		}

		newBootID, err := h.Reporter.BootID(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get boot id")
		}

		if newBootID == bootID {
			return errors.New("Boot id didn't change")
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 2 * time.Second,
	})
}

// Restore flashes the backup of the firmware to the DUT.
func (bs *BackupState) Restore(ctx context.Context, h *Helper) error {
	_, _, err := bs.Flash(ctx, h, bs.backupPath)
	if err != nil {
		return errors.Wrap(err, "failed to restore ec")
	}

	bs.ShouldRestoreFirmware = false
	return nil
}

// Flash flashes a file to the firmware. The file must already be on the DUT.
func (bs *BackupState) Flash(ctx context.Context, h *Helper, imagePath string) (hashBefore, hashAfter []byte, retErr error) {
	bs.ShouldRestoreFirmware = true
	testing.ContextLogf(ctx, "Flashing EC firmware: %q", imagePath)
	bootID, err := remote_dut.ReadBootID(ctx, bs.dut.Conn())
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get bootid")
	}

	if err := bs.dut.Conn().CommandContext(ctx, "bash", "-c", fmt.Sprintf("stdbuf -oL -eL nohup '%[1]s/ecflash.sh' '%[2]s' &>'%[1]s/ecflash.log' & exit", bs.RemoteTempDir(), imagePath)).Run(ssh.DumpLogOnError); err != nil {
		return nil, nil, errors.Wrap(err, "failed running ec flash")
	}
	h.CloseRPCConnection(ctx)

	testing.ContextLog(ctx, "Wait for reboot")
	if err := waitForReboot(ctx, bootID, h); err != nil {
		return nil, nil, errors.Wrap(err, "DUT didn't reboot")
	}
	out, err := linuxssh.ReadFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/ecflash.log", bs.RemoteTempDir()))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read ecflash.log")
	}
	if m := flashromExitCodeRe.FindSubmatch(out); m == nil || string(m[1]) != "0" {
		testing.ContextLogf(ctx, "ecflash.log:%s", out)
		return nil, nil, errors.Wrap(err, "flashrom failed")
	}
	m := hashBeforeRe.FindSubmatch(out)
	if m == nil {
		return nil, nil, errors.Errorf("failed to get hash before flash: %s", string(out))
	}
	hashBefore = m[1]
	m = hashAfterRe.FindSubmatch(out)
	if m == nil {
		return nil, nil, errors.Errorf("failed to get hash after flash: %s", string(out))
	}
	hashAfter = m[1]
	return hashBefore, hashAfter, nil
}

// BackupECFirmware makes a backup of the EC. Returns a BackupState object which must be closed.
// Caution: Set backupState.ShouldRestoreFirmware = true if you have corrupted the EC firmware in some way.
// Example:
//
//	backupState, err := firmware.BackupECFirmware(ctx, h)
//	if err != nil {
//		s.Fatal("Failed to backup: ", err)
//	}
//	defer func() {
//		retErr = errors.Join(backupState.Close(cleanupCtx, h), retErr)
//	}
func BackupECFirmware(ctx context.Context, h *Helper) (*BackupState, error) {
	backupState := BackupState{
		dut: h.DUT,
	}

	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/usr/local/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating remote temp dir")
	}
	backupState.remoteTempDir = strings.TrimSuffix(string(out), "\n")

	if err := linuxssh.WriteFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/ecflash.sh", backupState.RemoteTempDir()), []byte(shellScript), 0755); err != nil {
		return nil, errors.Join(errors.Wrap(err, "failed to write flash script"), backupState.Close(ctx, h))
	}

	testing.ContextLog(ctx, "Backup EC firmware")
	backupState.backupPath = fmt.Sprintf("%s/ec_backup.bin", backupState.remoteTempDir)
	if err := h.DUT.Conn().CommandContext(ctx, "flashrom", "-p", "ec", "-r", backupState.backupPath).Run(ssh.DumpLogOnError); err != nil {
		return nil, errors.Join(errors.Wrap(err, "failed taking ec backup"), backupState.Close(ctx, h))
	}
	return &backupState, nil
}
