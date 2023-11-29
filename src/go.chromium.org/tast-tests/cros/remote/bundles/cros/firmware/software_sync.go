// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
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
		Func: SoftwareSync,
		Desc: "Servo based EC software sync test",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_cr50"},
		// Don't run on EFS1 devices (fizz & kalista), there is a test firmware.ECUpdateID that tests those.
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnPlatform("fizz", "kalista")),
		Requirements: []string{"sys-fw-0022-v02"},
		Timeout:      15 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

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

func SoftwareSync(ctx context.Context, s *testing.State) {
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/usr/local/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed creating remote temp dir: ", err)
	}
	remoteTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		err := h.DUT.Conn().CommandContext(cleanupContext, "rm", "-rf", remoteTempDir).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed deleting remote temp dir: ", err)
		}
	}()
	s.Log("Backup EC firmware")
	if err := h.DUT.Conn().CommandContext(ctx, "flashrom", "-p", "ec", "-r", fmt.Sprintf("%s/ec_backup.bin", remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed taking ec backup: ", err)
	}
	if err := linuxssh.WriteFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/flash.sh", remoteTempDir), []byte(shellScript), 0755); err != nil {
		s.Fatal("Failed to write flash script: ", err)
	}

	shouldRestoreFirmware := false
	testing.ContextLog(ctx, "Get intial GBB flags")
	oldGBBFlags, err := fwCommon.GetGBBFlags(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed get gbb flags: ", err)
	}

	defer func() {
		if _, err := fwCommon.ClearAndSetGBBFlags(cleanupContext, h.DUT, oldGBBFlags); err != nil {
			s.Fatal("Failed to set gbb flag: ", err)
		}
		if shouldRestoreFirmware {
			restoreFirmware(cleanupContext, s, h, remoteTempDir, &shouldRestoreFirmware)
		}
	}()

	s.Log("Check DISABLE_EC_SOFTWARE_SYNC GBB flag is not set, if it is, clear it")
	if fwCommon.GBBFlagsContains(oldGBBFlags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
		testing.ContextLog(ctx, "Clearing GBB flag DISABLE_EC_SOFTWARE_SYNC")
		req := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}

		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req); err != nil {
			s.Fatal("Failed to set gbb flag: ", err)
		}
	}

	s.Log("Checking preconditions")
	// TODO(b/194910957): Old test checks that fw-a section does not have preamble flag PREAMBLE_USE_RO_NORMAL. this really needed?

	// Reboot just in case the firmware version we backed up isn't the same one that software sync will restore.
	// Use a cold reset to prevent "RO_AT_BOOT is not clear" errors.
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	checkActiveCopyRW(ctx, s, h.Servo)
	s.Log("Corrupt the EC section: ", bios.RWFWIDImageSection)
	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Error("Failed to get bootid: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", fmt.Sprintf("%s/ec_backup.bin", remoteTempDir), fmt.Sprintf("%s:%s/fwid.good", bios.RWFWIDImageSection, remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed extracting fwid.good: ", err)
	}
	if err := linuxssh.WriteFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/fwid.bad", remoteTempDir), []byte("invalid_version"), 0644); err != nil {
		s.Fatal("Failed to write fwid.bad: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "truncate", "-c", "-r", fmt.Sprintf("%s/fwid.good", remoteTempDir), fmt.Sprintf("%s/fwid.bad", remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed padding fwid.bad: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/ec_corrupt.bin", remoteTempDir), fmt.Sprintf("%s/ec_backup.bin", remoteTempDir), fmt.Sprintf("%s:%s/fwid.bad", bios.RWFWIDImageSection, remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed writing ec_corrupt.bin: ", err)
	}
	shouldRestoreFirmware = true
	if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", fmt.Sprintf("stdbuf -oL nohup '%[1]s/flash.sh' '%[1]s/ec_corrupt.bin' &>'%[1]s/corrupt.log' & exit", remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Error("Failed running flash.sh: ", err)
	}
	h.CloseRPCConnection(ctx)

	s.Log("Wait for reboot")
	if err := waitForReboot(ctx, bootID, h); err != nil {
		s.Error("DUT didn't reboot: ", err)
	}

	var ecHashBefore []byte
	out, err = linuxssh.ReadFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/corrupt.log", remoteTempDir))
	if err != nil {
		s.Fatal("Failed to read corrupt.log: ", err)
	}
	m := flashromExitCodeRe.FindSubmatch(out)
	if m == nil || string(m[1]) != "0" {
		s.Error("flashrom failed: ", string(out))
	}
	m = hashBeforeRe.FindSubmatch(out)
	if m == nil {
		s.Error("Failed to get hash before flash: ", string(out))
	} else {
		ecHashBefore = m[1]
	}
	m = hashAfterRe.FindSubmatch(out)
	if m == nil {
		s.Error("Failed to get hash after flash: ", string(out))
	} else {
		ecHashCorrupt := m[1]
		s.Logf("Checking that EC hash changed %q != %q", ecHashBefore, ecHashCorrupt)
		if bytes.Equal(ecHashCorrupt, ecHashBefore) {
			s.Fatalf("Flash failed, hash before == hash after: %s", string(out))
		}
	}

	s.Log("Expect EC in RW and RW is restored")
	checkECHash(ctx, s, ecHashBefore)
	checkActiveCopyRW(ctx, s, h.Servo)

	if features, err := h.DUT.Conn().CommandContext(ctx, "ectool", "inventory").Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to get features: ", err)
	} else if bytes.Contains(features, []byte("\n38 ")) { // EC_FEATURE_EFS2 == 38
		s.Log("Checking for NORMAL boot mode")
		if err := h.Servo.CheckGSCBootMode(ctx, []string{"NORMAL", "Verified"}); err != nil {
			s.Fatal("Incorrect boot mode: ", err)
		}

		s.Log("Corrupting ECRW hashcode in TPM kernel NV index")
		if err := h.Servo.RunCR50Command(ctx, "ec_comm corrupt"); err != nil {
			s.Fatal("Failed to corrupt ECRW hashcode: ", err)
		}
		s.Log("Reboot EC, verify RO, reboot AP, check hash")
		if err := ms.ModeAwareReboot(ctx, firmware.APOff, firmware.VerifyECRO, firmware.VerifyGSCNoBoot, firmware.WaitSoftwareSync); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
		h.CloseRPCConnection(ctx)

		s.Log("Checking for NORMAL boot mode")
		if err := h.Servo.CheckGSCBootMode(ctx, []string{"NORMAL", "Verified"}); err != nil {
			s.Fatal("Incorrect boot mode: ", err)
		}
		s.Log("Expect EC in RW and RW is restored")
		checkECHash(ctx, s, ecHashBefore)
		checkActiveCopyRW(ctx, s, h.Servo)
	}
}

func checkActiveCopyRW(ctx context.Context, s *testing.State, srvo *servo.Servo) {
	activeCopy := ""
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		activeCopy, err = srvo.GetString(ctx, servo.ECActiveCopy)
		return err
	}, &testing.PollOptions{
		Timeout: 20 * time.Second,
	})
	if err != nil {
		s.Fatal("EC active copy failed: ", err)
	}
	if !strings.HasPrefix(activeCopy, "RW") {
		s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
	}
}

func waitForReboot(ctx context.Context, bootID string, h *firmware.Helper) error {
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

func restoreFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, remoteTempDir string, shouldRestoreFirmware *bool) {
	s.Log("Restoring EC firmware")
	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Error("Failed to get bootid: ", err)
	}

	if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", fmt.Sprintf("stdbuf -oL -eL nohup '%[1]s/flash.sh' '%[1]s/ec_backup.bin' &>'%[1]s/restore.log' & exit", remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Error("Failed running ec restore: ", err)
	}
	h.CloseRPCConnection(ctx)

	s.Log("Wait for reboot")
	if err := waitForReboot(ctx, bootID, h); err != nil {
		s.Error("DUT didn't reboot: ", err)
	}
	if out, err := linuxssh.ReadFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/restore.log", remoteTempDir)); err != nil {
		s.Error("Failed to read restore.log: ", err)
	} else {
		m := flashromExitCodeRe.FindSubmatch(out)
		if m == nil || string(m[1]) != "0" {
			s.Error("flashrom failed: ", string(out))
		}
	}
	*shouldRestoreFirmware = false
}

const hashCommand = "ectool echash | grep hash: | sed \"s/hash:\\s\\+//\""

func checkECHash(ctx context.Context, s *testing.State, ecHashBefore []byte) {
	ecHashAfter, err := s.DUT().Conn().CommandContext(ctx, "sh", "-c", hashCommand).
		Output()
	if err != nil {
		s.Fatal("Failed to get ec hash: ", err)
	}
	ecHashAfter = bytes.TrimSuffix(ecHashAfter, []byte{'\n'})
	if !bytes.Equal(ecHashAfter, ecHashBefore) {
		s.Fatalf("EC hash wrong, got %q want %q", ecHashAfter, ecHashBefore)
	}
}
