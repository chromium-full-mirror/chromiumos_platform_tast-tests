// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
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
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.VbootCbfsIntegration()),
		Timeout:      30 * time.Minute,
		SoftwareDeps: []string{"chromeos_firmware", "crossystem", "flashrom"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode_a_file_header",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_attributes",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_length",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_loaded_in_romstage",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_file_loaded_in_ramstage",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "normal_mode_a_depthcharge_file",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, true},
			},
			{
				Name:    "normal_mode_b_file_header",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_attributes",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_length",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_loaded_in_romstage",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_file_loaded_in_ramstage",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "normal_mode_b_depthcharge_file",
				Fixture: fixture.NormalMode,
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, true},
			},
			{
				Name:    "dev_mode_a_file_header",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_attributes",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_length",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_loaded_in_romstage",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_file_loaded_in_ramstage",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, false},
			},
			{
				Name:    "dev_mode_a_depthcharge_file",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionA, true},
			},
			{
				Name:    "dev_mode_b_file_header",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/romstage", pb.CBFSCorruptType_MAGIC, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_attributes",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_ATTRIBUTES, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_length",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_LENGTH, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_loaded_in_romstage",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/ramstage", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_file_loaded_in_ramstage",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"fallback/payload", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, false},
			},
			{
				Name:    "dev_mode_b_depthcharge_file",
				Fixture: fixture.DevMode,
				Val:     apCorruptCBFSMetadataConfig{"ecrw.hash", pb.CBFSCorruptType_DATA, fwCommon.RWSectionB, true},
			},
		},
	})
}

func CorruptFWCBFS(ctx context.Context, s *testing.State) {
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

	s.Log("Backup firmware section: ", fwVariant)
	fwBackup, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{Section: sectionVariant, Programmer: pb.Programmer_BIOSProgrammer})
	if err != nil {
		s.Fatal("Failed to backup firmware section: ", fwVariant)
	}
	defer func(ctx context.Context) {
		h.DUT.Conn().CommandContext(ctx, "rm", fwBackup.Path).Output(ssh.DumpLogOnError)
	}(cleanupCtx)

	s.Log("Copy backup files to the Host")
	fwBackupHost, err := ioutil.TempFile("", "fwBackup")
	if err != nil {
		s.Fatal("Failed to create temporary file for firmware sign A backup")
	}
	defer os.Remove(fwBackupHost.Name())
	defer fwBackupHost.Close()

	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), fwBackup.Path, fwBackupHost.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy backup firmware from DUT to the Host: ", err)
	}

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
		if testConfig.RequireECSync {
			if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, oldGBBFlags); err != nil {
				s.Fatal("Failed to set gbb flag: ", err)
			}
		}

		// Disable WP so backup can be restored.
		if err := fwUtils.SetFWWriteProtect(ctx, h, false); err != nil {
			s.Fatal("Failed to set FW write protect state: ", err)
		}

		if err := h.RequireServo(ctx); err != nil {
			s.Fatal("Failed to init servo: ", err)
		}

		s.Log("Syncing TAST File from HOST")
		if err := h.SyncTastFilesToDUT(ctx); err != nil {
			s.Log(err, "syncing Tast files to DUT after booting to recovery")
		}

		// Require again here since reboots in test cause nil pointer errors otherwise.
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}

		s.Log("Get back FW Signs backup from host to DUT")
		if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), map[string]string{fwBackupHost.Name(): fwBackup.Path}, linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to get backup files to DUT from Host")
		}

		s.Log("Restore firmware section: ", fwVariant)
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
