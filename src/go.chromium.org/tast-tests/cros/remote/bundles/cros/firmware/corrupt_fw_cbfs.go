// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type apCorruptCBFSMetadataConfig struct {
	Filename        string
	CorruptType     pb.CBFSCorruptType
	FirmwareVariant fwCommon.RWSection
	RequireECSync   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptFWCBFS,
		Desc: "Servo based AP firmware CBFS file metadata and data corruption",
		Contacts: []string{
			"chromeos-faft@google.com",
			"czapiga@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level3"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.VbootCbfsIntegration()),
		Timeout:      30 * time.Minute,
		SoftwareDeps: []string{"chromeos_firmware", "crossystem", "flashrom"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode_a_file_header",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_attributes",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_length",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_loaded_in_romstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_loaded_in_ramstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_depthcharge_file",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, true},
			},
			{
				Name:    "normal_mode_b_file_header",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_attributes",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_length",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_loaded_in_romstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_loaded_in_ramstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_depthcharge_file",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, true},
			},
			{
				Name:    "dev_mode_a_file_header",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_attributes",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_length",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_loaded_in_romstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_loaded_in_ramstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_depthcharge_file",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, true},
			},
			{
				Name:    "dev_mode_b_file_header",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_attributes",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_length",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_loaded_in_romstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_loaded_in_ramstage",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_depthcharge_file",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevMode),
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, true},
			},
		},
	})
}

func CorruptFWCBFS(ctx context.Context, s *testing.State) {
	b := s.FixtValue().(*fixture.Value).BackupManager
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

	testConfig := s.Param().(apCorruptCBFSMetadataConfig)

	fwVariant := testConfig.FirmwareVariant
	var sectionVariant pb.ImageSection
	var fwVariantOpposite fwCommon.RWSection
	var rwSection fwCommon.RWSection
	if fwVariant == fwCommon.RWSectionA {
		sectionVariant = pb.ImageSection_FWBodyAImageSection
		fwVariantOpposite = fwCommon.RWSectionB
		rwSection = fwCommon.RWSectionA
	} else {
		sectionVariant = pb.ImageSection_FWBodyBImageSection
		fwVariantOpposite = fwCommon.RWSectionA
		rwSection = fwCommon.RWSectionB
	}

	// Reserve time for test cleanup in case test times out.
	cleanupCtx := ctx
	ctx, cancelCtx := ctxutil.Shorten(ctx, 15*time.Minute)
	defer cancelCtx()

	s.Log("Set the USB Mux direction to Host")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal(err, "failed to set the USB Mux direction to the Host")
	}

	testing.ContextLog(ctx, "Get intial GBB flags")
	oldGBBFlags, err := fwCommon.GetGBBFlags(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed get gbb flags: ", err)
	}

	defer func(ctx context.Context) {
		if err := h.RequireServo(ctx); err != nil {
			s.Fatal("Failed to init servo: ", err)
		}

		if testConfig.RequireECSync {
			if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, oldGBBFlags); err != nil {
				s.Fatal("Failed to set gbb flag: ", err)
			}
		}

		// Disable WP so backup can be restored.
		if err := fwUtils.SetFWWriteProtect(ctx, h, false); err != nil {
			s.Fatal("Failed to set FW write protect state: ", err)
		}

		backupOnDut := "/tmp/fwBackup.bin"
		if err := b.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, backupOnDut); err != nil {
			s.Fatal("Failed to copy backup firmware image to DUT: ", err)
		}

		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}

		s.Log("Restore firmware section: ", fwVariant)
		futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
		if err != nil {
			s.Fatal("Failed to create futility instance: ", err)
		}
		if _, err = futilityInstance.DumpFmapExtract(ctx, backupOnDut, map[string]string{
			"FW_MAIN_A": "/tmp/fwMainA.bin",
			"FW_MAIN_B": "/tmp/fwMainB.bin",
		}); err != nil {
			s.Fatal("Failed to extract section for recovery: ", err)
		}

		fwBackup := &pb.FWSectionInfo{
			Section:    sectionVariant,
			Programmer: pb.Programmer_BIOSProgrammer,
		}
		if sectionVariant == pb.ImageSection_FWBodyAImageSection {
			fwBackup.Path = "/tmp/fwMainA.bin"
		} else {
			fwBackup.Path = "/tmp/fwMainB.bin"
		}
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, fwBackup); err != nil {
			s.Fatalf("Failed to restore firmware section: %v. %v", fwVariant, err)
		}

		// Always go back to RW/A.
		if isFWSlotA, err := h.Reporter.CheckFWVersion(ctx, string(fwCommon.RWSectionA)); err != nil {
			s.Fatal("Failed to check firmware version: ", err)
		} else if isFWSlotA {
			// If finished in RW/A, then reboot just in case.
			testing.ContextLogf(ctx, "Set FW tries to %q", fwVariant)
			if err := firmware.SetFWTries(ctx, h.DUT, fwCommon.RWSectionA, 0); err != nil {
				s.Fatalf("Failed to set FW tries to %q: %q", fwVariant, err)
			}

			if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
				s.Fatal("Failed to perform mode aware reboot: ", err)
			}
		} else {
			if err := fwUtils.ChangeFWVariant(ctx, h, ms, fwCommon.RWSectionA); err != nil {
				s.Fatal("Failed to change FW variant: ", err)
			}
		}
	}(cleanupCtx)

	if testConfig.RequireECSync {
		s.Log("Check DISABLE_EC_SOFTWARE_SYNC GBB flag is not set, if it is, clear it")
		if fwCommon.GBBFlagsContains(oldGBBFlags, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC) {
			testing.ContextLog(ctx, "Clearing GBB flag DISABLE_EC_SOFTWARE_SYNC")
			req := pb.GBBFlagsState{Clear: []pb.GBBFlag{pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}

			if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req); err != nil {
				s.Fatal("Failed to set gbb flag: ", err)
			}
		}
	}

	// Always start from RW/A.
	if err := fwUtils.ChangeFWVariant(ctx, h, ms, fwCommon.RWSectionA); err != nil {
		s.Fatal("Failed to change FW variant: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	s.Log("Corrupt firmware body")
	if _, err := h.BiosServiceClient.CorruptCBFSFWSection(ctx, &pb.CBFSCorruptInfo{
		Filename:    testConfig.Filename,
		Type:        testConfig.CorruptType,
		SectionInfo: &pb.FWSectionInfo{Section: sectionVariant, Programmer: pb.Programmer_BIOSProgrammer},
	}); err != nil {
		s.Fatalf("Failed to corrupt CBFS file %q in section %q with method %q. Error: %v", testConfig.Filename, fwVariant, testConfig.CorruptType.String(), err)
	}

	testing.ContextLogf(ctx, "Set FW tries to %q", fwVariant)
	if err := firmware.SetFWTries(ctx, h.DUT, rwSection, 0); err != nil {
		s.Fatalf("Failed to set FW tries to %q: %q", fwVariant, err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	s.Log("Check the firmware version")
	if isFWVerOpposite, err := h.Reporter.CheckFWVersion(ctx, string(fwVariantOpposite)); err != nil {
		s.Fatal("Failed to check firmware version: ", err)
	} else if !isFWVerOpposite {
		s.Fatal("Failed to boot into the opposite firmware slot ", fwVariantOpposite)
	}
}
