// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
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
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Requirements: []string{"sys-fw-0022-v02"},
		Timeout:      15 * time.Minute,
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

const hashCommand = "ectool echash | grep hash: | sed \"s/hash:\\s\\+//\""

func SoftwareSync(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()
	restore, err := utils.EnableSoftwareSync(ctx, h, false)
	if err != nil {
		if restore != nil {
			s.Log("Failed to clear disable software sync flag: ", err)
			restore(cleanupContext, s)
		} else {
			s.Fatal("Failed to clear disable software sync flag: ", err)
		}
	}
	defer restore(cleanupContext, s)

	s.Log("Checking preconditions")
	// TODO(b/194910957): Old test checks that fw-a section does not have preamble flag PREAMBLE_USE_RO_NORMAL. this really needed?

	// Reboot just in case the firmware version we backed up isn't the same one that software sync will restore.
	// Use a cold reset to prevent "RO_AT_BOOT is not clear" errors.
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	h.CloseRPCConnection(ctx)

	activeCopy, err := getActiveCopy(ctx, h.Servo)
	if err != nil {
		s.Fatal("EC active copy failed: ", err)
	}
	if !strings.HasPrefix(activeCopy, "RW") {
		s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
	}
	ecHashBefore, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", hashCommand).
		Output()
	if err != nil {
		s.Fatal("Failed to get ec hash: ", err)
	}
	ecSection := pb.ImageSection_ECRWImageSection
	if activeCopy == "RW_B" {
		ecSection = pb.ImageSection_ECRWBImageSection
	}
	s.Log("Corrupt the EC section: ", ecSection)
	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}
	if _, err = h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{Section: ecSection, Programmer: pb.Programmer_ECProgrammer}); err != nil {
		s.Fatal("Failed to corrupt EC: ", err)
	}

	// Tell the EC to recalculate the hash
	if err := h.DUT.Conn().CommandContext(ctx, "ectool", "echash", "start", "rw").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("EC hash start failed: ", err)
	}
	ecHashCorrupt, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", hashCommand).Output()
	if err != nil {
		s.Fatal("Failed to get ec hash: ", err)
	}
	if bytes.Equal(ecHashCorrupt, ecHashBefore) {
		s.Fatal("EC hash unchanged, corruption step failed")
	}

	s.Log("Reboot AP, check EC hash, and software sync it")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.WaitSoftwareSync, firmware.AllowGBBForce); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	h.CloseRPCConnection(ctx)

	s.Log("Expect EC in RW and RW is restored")
	ecHashAfter, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", hashCommand).
		Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get ec hash: ", err)
	}
	if !bytes.Equal(ecHashAfter, ecHashBefore) {
		s.Fatalf("EC hash wrong, got %s want %s", ecHashAfter, ecHashBefore)
	}
	activeCopy, err = getActiveCopy(ctx, h.Servo)
	if err != nil {
		s.Fatal("EC active copy failed: ", err)
	}
	if !strings.HasPrefix(activeCopy, "RW") {
		s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
	}

	if features, err := h.DUT.Conn().CommandContext(ctx, "ectool", "inventory").Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to get features: ", err)
	} else if bytes.Contains(features, []byte("\n38 ")) {
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
		ecHashAfter, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", hashCommand).
			Output()
		if err != nil {
			s.Fatal("Failed to get ec hash: ", err)
		}
		if !bytes.Equal(ecHashAfter, ecHashBefore) {
			s.Fatalf("EC hash wrong, got %s want %s", ecHashAfter, ecHashBefore)
		}
		activeCopy, err = getActiveCopy(ctx, h.Servo)
		if err != nil {
			s.Fatal("EC active copy failed: ", err)
		}
		if !strings.HasPrefix(activeCopy, "RW") {
			s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
		}
	}
}

func getActiveCopy(ctx context.Context, s *servo.Servo) (string, error) {
	activeCopy := ""
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		activeCopy, err = s.GetString(ctx, servo.ECActiveCopy)
		return err
	}, &testing.PollOptions{
		Timeout: 20 * time.Second,
	})
	return activeCopy, err
}
