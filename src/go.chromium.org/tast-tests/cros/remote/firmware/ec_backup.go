// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	remote_dut "go.chromium.org/tast-tests/cros/remote/dut"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
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
until ectool echash | grep 'done' ; do sleep 1 ; done
echo -n "AFTER "
ectool echash | grep "hash:"
sync
ectool reboot_ec
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

func waitForReboot(ctx context.Context, bootID string, h *Helper, opts ...WaitConnectOption) error {
	wcOpts := append([]WaitConnectOption{ResetEthernetDongle}, opts...)
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := h.WaitConnect(ctx, wcOpts...); err != nil {
			return errors.Wrap(err, "failed to connect")
		}

		newBootID, err := h.Reporter.BootID(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get boot id")
		}

		if newBootID == bootID {
			return errors.Errorf("Boot id didn't change, got %v", newBootID)
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 2 * time.Second,
	})
}

var ecConsoleDigestRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func readECRWHashOnDUT(ctx context.Context, d *dut.DUT) ([]byte, error) {
	ec := NewECTool(d, ECToolNameMain)
	if err := ec.Command(ctx, "echash", "start", "rw").Run(ssh.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "starting EC RW hash")
	}
	var digest []byte
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		info, err := ec.Hash(ctx)
		if err != nil {
			return err
		}
		if info.Status != "done" {
			return errors.Errorf("echash status is %q, want done", info.Status)
		}
		digest = []byte(info.Hash)
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		return nil, errors.Wrap(err, "waiting for EC RW hash")
	}
	return digest, nil
}

// FlashECViaServo copies dutImagePath from the DUT to the Servo host, flashes
// it with flash_ec while keeping the AP off in G3 (--try_apshutdown) so early
// AP software sync cannot overwrite EC RW, and returns the post-flash EC RW
// SHA-256 hex digest read from the EC console.
func FlashECViaServo(ctx context.Context, h *Helper, dutImagePath string) (hashAfter []byte, retErr error) {
	if err := h.RequireServo(ctx); err != nil {
		return nil, errors.Wrap(err, "requiring servo")
	}
	localTmpDir, err := os.MkdirTemp("", "ecflash-*")
	if err != nil {
		return nil, errors.Wrap(err, "creating local temp dir")
	}
	defer func() {
		retErr = errors.Join(retErr, os.RemoveAll(localTmpDir))
	}()
	localImagePath := filepath.Join(localTmpDir, "ec.bin")
	if err := linuxssh.GetFile(ctx, h.DUT.Conn(), dutImagePath, localImagePath, linuxssh.DereferenceSymlinks); err != nil {
		return nil, errors.Wrap(err, "copying EC image from DUT")
	}

	servoTmpOut, err := h.ServoProxy.OutputCommand(ctx, true, "mktemp", "-d", "/tmp/ecflash.XXXXXX")
	if err != nil {
		return nil, errors.Wrap(err, "creating servo temp dir")
	}
	servoTmpDir := strings.TrimSpace(string(servoTmpOut))
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if err := h.ServoProxy.RunCommand(rmCtx, true, "rm", "-rf", servoTmpDir); err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "removing servo temp dir"))
		}
	}()
	servoImagePath := filepath.Join(servoTmpDir, "ec.bin")
	if err := h.ServoProxy.PutFiles(ctx, true, map[string]string{localImagePath: servoImagePath}); err != nil {
		return nil, errors.Wrap(err, "copying EC image to servo host")
	}

	if err := h.CloseRPCConnection(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to close RPC connection before flash_ec: ", err)
	}

	args := []string{
		fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()),
		fmt.Sprintf("--image=%s", servoImagePath),
		"--verify",
		"--try_apshutdown",
	}
	var flashErr error
	for attempt := 1; attempt <= 2; attempt++ {
		flashErr = h.ServoProxy.RunCommand(ctx, true, "flash_ec", args...)
		if flashErr == nil {
			break
		}
		testing.ContextLogf(ctx, "flash_ec attempt %d failed: %v", attempt, flashErr)
	}
	if flashErr != nil {
		return nil, errors.Wrap(flashErr, "running flash_ec on servo host")
	}
	if err := h.RestoreServoPower(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to restore servo power after flash_ec: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Servo.RunECCommandGetOutput(ctx, "hash", []string{`Digest:\s+(\([^)]+\)|[0-9a-f]{64})`})
		if err != nil {
			return errors.Wrap(err, "reading hash from EC console")
		}
		val := strings.TrimSpace(out[0][1])
		if !ecConsoleDigestRe.MatchString(val) {
			if val == "(invalid)" || val == "(aborting)" {
				_ = h.Servo.RunECCommand(ctx, "hash rw")
			}
			return errors.Errorf("EC hash not ready: %s", val)
		}
		hashAfter = []byte(val)
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return nil, errors.Wrap(err, "polling post-flash EC hash over servo")
	}
	return hashAfter, nil
}

func ecSectionMatches(ctx context.Context, d *dut.DUT, currPath, backupPath, workDir, section string) (bool, error) {
	currSec := filepath.Join(workDir, fmt.Sprintf("curr_%s.bin", section))
	backupSec := filepath.Join(workDir, fmt.Sprintf("backup_%s.bin", section))
	defer func() {
		_ = d.Conn().CommandContext(ctx, "rm", "-f", currSec, backupSec).Run()
	}()
	if err := d.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", currPath, fmt.Sprintf("%s:%s", section, currSec)).Run(ssh.DumpLogOnError); err != nil {
		return false, errors.Wrapf(err, "extracting %s from current EC image", section)
	}
	if err := d.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", backupPath, fmt.Sprintf("%s:%s", section, backupSec)).Run(ssh.DumpLogOnError); err != nil {
		return false, errors.Wrapf(err, "extracting %s from backup EC image", section)
	}
	if err := d.Conn().CommandContext(ctx, "cmp", "-s", currSec, backupSec).Run(); err != nil {
		return false, nil
	}
	return true, nil
}

// Restore restores the backed-up EC firmware to the DUT. When EC software sync
// is enabled in GBB flags, Restore avoids writing EC_RW directly and instead
// only restores WP_RO in-place (if WP_RO changed) and cold-reboots so AP EC
// software sync restores EC_RW.
func (bs *BackupState) Restore(ctx context.Context, h *Helper) error {
	if !bs.dut.Connected(ctx) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			return errors.Wrap(err, "ensuring DUT booted before EC restore")
		}
	}

	gbbFlags, gbbErr := fwCommon.GetGBBFlags(ctx, bs.dut)
	if gbbErr == nil && !fwCommon.GBBFlagsContains(gbbFlags, fwpb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
		ms, err := NewModeSwitcher(ctx, h)
		if err != nil {
			return errors.Wrap(err, "creating mode switcher for EC restore")
		}
		if err := h.RequireServo(ctx); err == nil {
			if activeCopy, err := h.Servo.GetString(ctx, servo.ECActiveCopy); err == nil && !strings.HasPrefix(activeCopy, "RW") {
				testing.ContextLogf(ctx, "EC active copy is %q with software sync enabled; cold-rebooting into RW before checking WP_RO", activeCopy)
				if err := ms.ModeAwareReboot(ctx, ColdReset); err != nil {
					return errors.Wrap(err, "cold-rebooting EC into RW")
				}
			}
		}

		var flashromCfg flashrom.Config
		flashromInst, probeCtx, shutdown, _, err := flashromCfg.
			FlashromInit(flashrom.VerbosityInfo).
			ProgrammerInit(flashrom.ProgrammerEc, "").
			SetDut(bs.dut).
			Probe(ctx)
		if err != nil {
			return errors.Join(errors.Wrap(err, "probing EC flashrom for restore"), shutdown())
		}

		currPath := filepath.Join(bs.remoteTempDir, "ec_current.bin")
		if _, err := flashromInst.Read(probeCtx, currPath, nil); err != nil {
			return errors.Join(errors.Wrap(err, "reading current EC firmware"), shutdown())
		}
		defer func() {
			_ = bs.dut.Conn().CommandContext(ctx, "rm", "-f", currPath).Run()
		}()

		wpROMatches, err := ecSectionMatches(probeCtx, bs.dut, currPath, bs.backupPath, bs.remoteTempDir, "WP_RO")
		if err != nil {
			return errors.Join(err, shutdown())
		}
		ecRWMatches, err := ecSectionMatches(probeCtx, bs.dut, currPath, bs.backupPath, bs.remoteTempDir, "EC_RW")
		if err != nil {
			return errors.Join(err, shutdown())
		}

		needReboot := !ecRWMatches
		if !wpROMatches {
			testing.ContextLog(probeCtx, "EC software sync is enabled; restoring only WP_RO via flashrom")
			if err := h.SetECWriteProtect(probeCtx, false); err != nil {
				return errors.Join(errors.Wrap(err, "disabling EC write protect before WP_RO restore"), shutdown())
			}
			if _, err := flashromInst.Write(probeCtx, bs.backupPath, true, false, "", []string{"WP_RO"}); err != nil {
				return errors.Join(errors.Wrap(err, "restoring EC WP_RO via flashrom"), shutdown())
			}
			needReboot = true
		}
		if err := shutdown(); err != nil {
			return errors.Wrap(err, "shutting down EC flashrom after restore")
		}
		if needReboot {
			testing.ContextLog(ctx, "Cold-rebooting DUT so AP EC software sync restores EC_RW")
			if err := ms.ModeAwareReboot(ctx, ColdReset); err != nil {
				return errors.Wrap(err, "cold-rebooting DUT to sync EC_RW")
			}
		}
		bs.ShouldRestoreFirmware = false
		return nil
	}

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

	losesPower, err := h.LosesPowerOnECReset(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "checking if DUT loses power on EC reset")
	}
	hasExecInRAM := false
	if !losesPower {
		hasExecInRAM, err = NewECTool(bs.dut, ECToolNameMain).HasExecInRAM(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "checking EC_FEATURE_EXEC_IN_RAM")
		}
	}
	if losesPower || !hasExecInRAM {
		hashBefore, err = readECRWHashOnDUT(ctx, bs.dut)
		if err != nil {
			return nil, nil, err
		}
		hashAfter, err = FlashECViaServo(ctx, h, imagePath)
		if err != nil {
			return nil, nil, err
		}
		h.PowerOnDUT(ctx)
		testing.ContextLog(ctx, "Wait for reboot after servo EC flash")
		if err := waitForReboot(ctx, bootID, h, SkipPDRoleSnk); err != nil {
			return nil, nil, errors.Wrap(err, "DUT didn't reboot after servo EC flash")
		}
		return hashBefore, hashAfter, nil
	}

	if err := bs.dut.Conn().CommandContext(ctx, "bash", "-c", fmt.Sprintf("stdbuf -oL -eL nohup '%[1]s/ecflash.sh' '%[2]s' &>'%[1]s/ecflash.log' & exit", bs.RemoteTempDir(), imagePath)).Run(ssh.DumpLogOnError); err != nil {
		return nil, nil, errors.Wrap(err, "failed running ec flash")
	}
	h.CloseRPCConnection(ctx)

	testing.ContextLog(ctx, "Wait for reboot")
	if err := waitForReboot(ctx, bootID, h); err != nil {
		retErr = errors.Wrap(err, "DUT didn't reboot")
	}
	out, err := linuxssh.ReadFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/ecflash.log", bs.RemoteTempDir()))
	if err != nil {
		return nil, nil, errors.Join(retErr, errors.Wrap(err, "failed to read ecflash.log"))
	}
	if m := flashromExitCodeRe.FindSubmatch(out); m == nil || string(m[1]) != "0" {
		testing.ContextLogf(ctx, "ecflash.log:%s", out)
		return nil, nil, errors.Join(retErr, errors.Wrap(err, "flashrom failed"))
	}
	m := hashBeforeRe.FindSubmatch(out)
	if m == nil {
		return nil, nil, errors.Join(retErr, errors.Errorf("failed to get hash before flash: %s", string(out)))
	}
	hashBefore = m[1]
	m = hashAfterRe.FindSubmatch(out)
	if m == nil {
		return nil, nil, errors.Join(retErr, errors.Errorf("failed to get hash after flash: %s", string(out)))
	}
	hashAfter = m[1]
	return hashBefore, hashAfter, retErr
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
	var flashromCfg flashrom.Config
	flashromInst, probeCtx, shutdown, _, err := flashromCfg.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerEc, "").
		SetDut(h.DUT).
		Probe(ctx)
	if err != nil {
		return nil, errors.Join(errors.Wrap(err, "probing EC flashrom"), shutdown(), backupState.Close(ctx, h))
	}
	if _, err := flashromInst.Read(probeCtx, backupState.backupPath, nil); err != nil {
		return nil, errors.Join(errors.Wrap(err, "failed taking ec backup"), shutdown(), backupState.Close(ctx, h))
	}
	if err := shutdown(); err != nil {
		return nil, errors.Join(errors.Wrap(err, "shutting down EC flashrom"), backupState.Close(ctx, h))
	}
	return &backupState, nil
}
