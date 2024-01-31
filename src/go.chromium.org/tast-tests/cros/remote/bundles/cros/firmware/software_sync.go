// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
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

	backupState, err := firmware.BackupECFirmware(ctx, h)
	if err != nil {
		s.Fatal("Failed to backup: ", err)
	}
	defer func() {
		err := backupState.Close(cleanupContext, h)
		if err != nil {
			s.Fatal("Failed to cleanup EC backup: ", err)
		}
	}()

	testing.ContextLog(ctx, "Get intial GBB flags")
	oldGBBFlags, err := fwCommon.GetGBBFlags(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed get gbb flags: ", err)
	}

	defer func() {
		if _, err := fwCommon.ClearAndSetGBBFlags(cleanupContext, h.DUT, oldGBBFlags); err != nil {
			s.Fatal("Failed to set gbb flag: ", err)
		}
	}()

	s.Log("Check DISABLE_EC_SOFTWARE_SYNC GBB flag is not set, if it is, clear it")
	if fwCommon.GBBFlagsContains(oldGBBFlags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
		backupState.ShouldRestoreFirmware = true
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
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", fmt.Sprintf("%s/ec_backup.bin", backupState.RemoteTempDir()), fmt.Sprintf("%s:%s/fwid.good", bios.RWFWIDImageSection, backupState.RemoteTempDir())).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed extracting fwid.good: ", err)
	}
	if err := linuxssh.WriteFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/fwid.bad", backupState.RemoteTempDir()), []byte("invalid_version"), 0644); err != nil {
		s.Fatal("Failed to write fwid.bad: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "truncate", "-c", "-r", fmt.Sprintf("%s/fwid.good", backupState.RemoteTempDir()), fmt.Sprintf("%s/fwid.bad", backupState.RemoteTempDir())).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed padding fwid.bad: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/ec_corrupt.bin", backupState.RemoteTempDir()), fmt.Sprintf("%s/ec_backup.bin", backupState.RemoteTempDir()), fmt.Sprintf("%s:%s/fwid.bad", bios.RWFWIDImageSection, backupState.RemoteTempDir())).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed writing ec_corrupt.bin: ", err)
	}
	ecHashBefore, ecHashCorrupt, err := backupState.Flash(ctx, h, fmt.Sprintf("%s/ec_corrupt.bin", backupState.RemoteTempDir()))
	if err != nil {
		s.Fatal("Failed to corrupt ec: ", err)
	}
	s.Logf("Checking that EC hash changed %q != %q", ecHashBefore, ecHashCorrupt)
	if bytes.Equal(ecHashCorrupt, ecHashBefore) {
		s.Fatalf("Flash failed, hash before == hash after: %s", string(ecHashCorrupt))
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
