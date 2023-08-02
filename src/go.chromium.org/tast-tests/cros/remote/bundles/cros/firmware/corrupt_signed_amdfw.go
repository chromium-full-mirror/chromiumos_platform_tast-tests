// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/signedamdfw"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CorruptSignedAMDFW,
		Desc:         "Servo based Signed AMDFW section corruption test",
		Contacts:     []string{"chromeos-faft@google.com", "kramasub@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Timeout:      20 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"crossystem", "flashrom", "amd_cpu"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		Params: []testing.Param{
			{
				Name:    "a_normal_mode",
				Fixture: fixture.NormalMode,
				Val:     "A",
			},
			{
				Name:    "b_normal_mode",
				Fixture: fixture.NormalMode,
				Val:     "B",
			},
			{
				Name:    "a_dev_mode",
				Fixture: fixture.DevModeGBB,
				Val:     "A",
			},
			{
				Name:    "b_dev_mode",
				Fixture: fixture.DevModeGBB,
				Val:     "B",
			},
		},
	})
}

func CorruptSignedAMDFW(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if ok := signedamdfw.CheckForSignedAMDFWSection(ctx, s, h); !ok {
		s.Log("Skipping test since SIGNED_AMDFW* section doesnot exist")
		return
	}

	fwVariant := s.Param().(string)
	var fwVariantOpposite string
	var sectionVariant pb.ImageSection
	var fwRWSection fwCommon.RWSection
	if fwVariant == "A" {
		fwVariantOpposite = "B"
		sectionVariant = pb.ImageSection_SignedAMDFWAImageSection
		fwRWSection = fwCommon.RWSectionA
	} else {
		fwVariantOpposite = "A"
		sectionVariant = pb.ImageSection_SignedAMDFWBImageSection
		fwRWSection = fwCommon.RWSectionB
	}

	s.Logf("Backup SIGNED_AMDFW_%s", fwVariant)
	AMDFWBkp, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{Section: sectionVariant, Programmer: pb.Programmer_BIOSProgrammer})
	if err != nil {
		s.Fatalf("Failed to backup SIGNED_AMDFW_%s region: %v", fwVariant, err)
	}

	defer func() {
		s.Logf("Delete AMDFW backup %s", AMDFWBkp.Path)
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", AMDFWBkp.Path).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete AMDFW backup: ", err)
		}
	}()

	s.Log("Set the USB Mux direction to Host")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal(err, "failed to set the USB Mux direction to the Host")
	}

	if err := fwUtils.ChangeFWVariant(ctx, h, ms, fwRWSection); err != nil {
		s.Fatalf("Failed to change FW variant %s: %v", fwVariant, err)
	}

	// Restore AMDFW
	defer func() {
		// Disable wp so backup can be restored.
		if err := fwUtils.SetFWWriteProtect(ctx, h, false); err != nil {
			s.Fatal("Failed to set FW write protect state: ", err)
		}

		if err := h.RequireServo(ctx); err != nil {
			s.Fatal("Failed to init servo: ", err)
		}

		// Require again here since reboots in test cause nil pointer errors otherwise.
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}

		s.Log("Restore AMDFW")
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, AMDFWBkp); err != nil {
			s.Fatal("Failed to restore Signed AMDFW: ", err)
		}

		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to perform mode aware reboot: ", err)
		}

		if isFWVerCorrect, err := h.Reporter.CheckFWVersion(ctx, fwVariantOpposite); err != nil {
			s.Fatal(err, "failed to check a firmware version")
		} else if !isFWVerCorrect {
			s.Fatal("Failed to boot into the opposite firmware")
		}

	}()

	s.Logf("Corrupt SIGNED_AMDFW_%s", fwVariant)
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{Section: sectionVariant, Programmer: pb.Programmer_BIOSProgrammer}); err != nil {
		s.Fatalf("Failed to corrupt Firmware Body %s section: %v", fwVariant, err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	s.Log("Check the firmware version")
	if isFWVerOpp, err := h.Reporter.CheckFWVersion(ctx, fwVariantOpposite); err != nil {
		s.Fatal(err, "failed to check a firmware version")
	} else if !isFWVerOpp {
		s.Fatalf("Failed to boot into the opposite firmware %s", fwVariantOpposite)
	}
}
