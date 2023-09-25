// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"

	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
)

var (
	// board is a runtime variable to store board information
	// of dut.
	board = testing.RegisterVarString(
		"firmware.board",
		"",
		"A variable to store the board information for the dut")

	// branch is a runtime variable to store which branch to download firmware from
	// for the dut.
	branch = testing.RegisterVarString(
		"firmware.branch",
		"",
		"A variable to store the branch information for the fw download")

	// branch is a runtime variable to store which version of firmware to download
	// for the dut.
	firmwareVersion = testing.RegisterVarString(
		"firmware.firmwareVersion",
		"",
		"A variable to store the version information for the fw download")

	// GCS location for the firmware to be downloaded
	firmwarePath = testing.RegisterVarString(
		"firmware.firmwarePath",
		"",
		"A variable to store the path information for the fw download")
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateDutFirmwareServo,
		Desc: "Update AP and EC firmware from Servo",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      90 * time.Minute, // 1hr30min.
	})
}

// UpdateDutFirmwareServo reads the current AP firmware and flashes it back from the servo using futility
func UpdateDutFirmwareServo(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	firmwarePathVal := string(firmwarePath.Value())

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// Confirm the CCD is open.
	hasCCD, err := h.Servo.HasCCD(ctx)
	if err != nil {
		s.Fatal("Failed while checking if servo has a CCD connection: ", err)
	}
	if hasCCD {
		if val, err := h.Servo.GetString(ctx, servo.GSCCCDLevel); err != nil {
			s.Fatal("Failed to get gsc_ccd_level: ", err)
		} else if val != servo.Open {
			s.Logf("CCD is not open, got %q. Attempting to unlock", val)
			if err := h.Servo.SetString(ctx, servo.CR50Testlab, servo.Open); err != nil {
				s.Fatal("Failed to unlock CCD: ", err)
			}
		}
	}

	s.Log("Disabling hardware write protect")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to disable hardware write protect: ", err)
	}
	s.Log("Disabling software write protect")
	if err := h.ServoProxy.RunCommand(ctx, true, "futility", "flash", "--wp-disable", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort())); err != nil {
		s.Fatalf("write protect disable failed at %q", err)
	}
	s.Log("Disabling software write protect completed")

	// Check that the DUT is booted after disabling write protect
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	uuid, _ := uuid.NewRandom()
	const (
		tmpFirmwareDir        = "/mnt/stateful_partition/tmp"
		backupFirmwareFile    = "backupfw.bin"
		ecFirmwareFileToFlash = "ecFirmwareForTest.bin"
		apFirmwareFileToFlash = "FirmwareForTest.bin"
	)
	servoTmpDir := fmt.Sprintf("%s-%s", tmpFirmwareDir, uuid)
	if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", servoTmpDir); err != nil {
		s.Fatal("Failed to create temp directory for saving existing firmware: ", err)
	}
	// Delete the tmp directory on the servo at the end
	defer func() {
		s.Log("Deleting tmp directory on servo: ", servoTmpDir)
		if err := h.ServoProxy.RunCommand(ctx, false, "rm", "-rf", servoTmpDir); err != nil {
			s.Fatal("Failed to delete temp directory for saving existing firmware: ", err)
		}
	}()

	// Get the initial fwid from 'crossystem fwid'.
	initialRwFwid, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		s.Fatal("Failed to get crossystem fwid: ", err)
	}
	re := regexp.MustCompile(`Google_([a-z-A-Z]*)\.(\d*\.\d*.\d*)`)
	match := re.FindStringSubmatch(initialRwFwid)
	if len(match) != 3 {
		s.Fatalf("Unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, initialRwFwid)
	}
	fwidModel := strings.ToLower(match[1])
	initialRwFwid = match[2]

	// Get the RO firmware version ID available on the DUT.
	initialROFwid, err := fwUtils.GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		s.Fatal("Failed to get AP RO ID: ", err)
	}

	s.Log("Backing up AP firmware")
	if err := h.ServoProxy.RunCommand(ctx, false, "futility", "read", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile)); err != nil {
		s.Fatal("Failed to read fw using futility: ", err)
	}

	s.Log("Completed backup of existing fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}
	tmpDir, err := os.MkdirTemp("", "firmware-UpdateDUTFirmwareServo")
	if err != nil {
		s.Fatal("Failed to create a new directory for the test: ", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	// Backup EC Firmware
	testing.ContextLog(ctx, "Backing up current EC_RW")
	backupRW, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    pb.ImageSection_ECRWImageSection,
		Programmer: pb.Programmer_ECProgrammer,
		Path:       "/usr/local/share/tast/",
	})
	if err != nil {
		s.Fatal("Failed to backup EC RW firmware: ", err)
	}
	testing.ContextLog(ctx, "Backing up current EC_RO")
	backupRO, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    pb.ImageSection_ECROImageSection,
		Programmer: pb.Programmer_ECProgrammer,
		Path:       "/usr/local/share/tast/",
	})
	if err != nil {
		s.Fatal("Failed to backup EC RO firmware: ", err)
	}

	// Flash DUT with the initial fw at the end
	defer func() {
		s.Log("Flashing DUT with backup AP firmware file")
		if err := h.ServoProxy.RunCommand(ctx, false, "futility", "update", "-i", fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile), fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), "--gbb_flags=0x18"); err != nil {
			s.Fatal("Failed to flash DUT bin file: ", err)
		}
		s.Log("Completed flashing of backup AP fw")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
		}

		// Verify RO/RW firmware versions are the prior ones after flashing.
		// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
		if err = fwUtils.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
			s.Fatal("Failed while verifying firmware IDs after flashing at the end of test: ", err)
		}

		s.Log("Flashing DUT with backup EC RW firmware file: ", backupRW)
		h.DisconnectDUT(ctx)
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Can't restore firmware, DUT is off: ", err)
		}
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, backupRW); err != nil {
			s.Fatal("Failed to restore EC firmware: ", err)
		}

		s.Log("Flashing DUT with backup EC RO firmware file: ", backupRO)
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}
		s.Log("Restoring EC firmware backup using: ", backupRO)
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, backupRO); err != nil {
			s.Fatal("Failed to restore EC firmware: ", err)
		}

		// Reboot and check active copy after restore.
		ms, err := firmware.NewModeSwitcher(ctx, h)
		if err != nil {
			s.Fatal("Creating mode switcher: ", err)
		}
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
		s.Log("Checking ec_active_copy is RW or RW_B")
		activeCopy, err := h.Servo.GetString(ctx, "ec_active_copy")
		if err != nil {
			s.Fatal("EC active copy failed: ", err)
		}
		if !strings.HasPrefix(activeCopy, "RW") {
			s.Fatalf("EC active copy incorrect, got %q want RW", activeCopy)
		}
	}()

	s.Log("Downloading EC Firmware to Flash")
	if firmwarePathVal == "" {
		return
	}

	ecBinToFlash := downloadFirmwareFromGCS(ctx, s, tmpDir, firmwarePathVal, fwidModel, fwUtils.ECFirmware)
	s.Log("EC Firmware to Flash: ", ecBinToFlash)

	if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash): fmt.Sprintf("%s/%s", servoTmpDir, ecFirmwareFileToFlash)}); err != nil {
		s.Fatal("Failed to copy files to servo host: ", err)
	}

	// Flash EC
	s.Log("Flashing DUT EC with downloaded firmware file")
	ecChip, err := h.Servo.GetString(ctx, servo.ECChip)
	if err != nil {
		s.Fatal("Failed to read DUT EC Chip: ", err)
	}
	if ecChip == "stm32" {
		if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, ecFirmwareFileToFlash), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--bitbang_rate=57600", "--verify", "--verbose"); err != nil {
			s.Fatal("Failed to flash EC firmware bin file: ", err)
		}
	} else if !strings.HasPrefix(ecChip, "it8") {
		// Flashing blocked for ite chips due to b/268108518
		if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, ecFirmwareFileToFlash), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--verify", "--verbose"); err != nil {
			s.Fatal("Failed to flash EC firmware bin file: ", err)
		}
	}
	s.Log("Completed flashing of downloaded ec fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	apBinToFlash := downloadFirmwareFromGCS(ctx, s, tmpDir, firmwarePathVal, fwidModel, fwUtils.APFirmware)
	s.Log("AP Firmware to Flash: ", apBinToFlash)

	if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{fmt.Sprintf("%s/%s", tmpDir, apBinToFlash): fmt.Sprintf("%s/%s", servoTmpDir, apFirmwareFileToFlash)}); err != nil {
		s.Fatal("Failed to copy files to servo host: ", err)
	}

	s.Log("Flashing DUT AP with downloaded firmware file")
	if err := h.ServoProxy.RunCommand(ctx, false, "futility", "update", "-i", fmt.Sprintf("%s/%s", servoTmpDir, apFirmwareFileToFlash), fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), "--gbb_flags=0x18"); err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err)
	}
	s.Log("Completed flashing of downloaded fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// Verify RO/RW firmware versions are the downloaded firmware versions after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err = fwUtils.VerifyFwIDs(ctx, h, firmwarePathVal, firmwarePathVal); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}

}

// downloadFirmwareFromGCS reads a file from GCS based on the board, branch and firmware version specified
func downloadFirmwareFromGCS(ctx context.Context, s *testing.State, tmpDir, firmwareFilepath, model string, fwType fwUtils.FirmwareType) string {
	// Download the latest shipped firmware.
	if err := fwUtils.DownloadFirmwareFile(ctx, s, tmpDir, firmwareFilepath); err != nil {
		s.Fatal("Failed while downloading file: ", err)
	}
	// Untar the binary file with respect to the model name found in 'crossystem fwid'.
	binToFlash, err := fwUtils.UntarUnknownFileName(ctx, tmpDir, model, fwType)
	s.Log("Bin to Flash: ", binToFlash)
	if err != nil {
		s.Fatal("Failed to untar file: ", err)
	}
	return binToFlash
}
