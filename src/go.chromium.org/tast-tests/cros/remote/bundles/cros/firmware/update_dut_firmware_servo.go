// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"

	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
)

const (
	tmpServoFirmwareDir       = "/mnt/stateful_partition/tmp"
	alternateServoFirmwareDir = "/var/tmp"
	tmpDutFirmwareDir         = "/mnt/stateful_partition/unencrypted/preserve/firmware"
	backupFirmwareFile        = "backupfw.bin"
	ecFirmwareFileToFlash     = "ecFirmwareForTest.bin"
	apFirmwareFileToFlash     = "FirmwareForTest.bin"
	monitorFileToFlash        = "npcx_monitor.bin"
)

var (
	// GCS location for the firmware to be downloaded
	firmwarePath = testing.RegisterVarString(
		"firmware.firmwarePath",
		"",
		"A variable to store the path information for the fw download")

	// Local location for the downloaded firmware
	localFirmwarePath = testing.RegisterVarString(
		"firmware.localFirmwarePath",
		"",
		"A variable to store the local path information for the download firmware")
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
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService", "dutfs.ServiceName"},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      90 * time.Minute, // 1hr30min.
	})
}

// UpdateDutFirmwareServo reads the current AP firmware and flashes it back from the servo using futility
func UpdateDutFirmwareServo(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	firmwarePathVal := string(firmwarePath.Value())
	localFirmwarePathVal := string(localFirmwarePath.Value())

	if firmwarePathVal != "" && localFirmwarePathVal != "" {
		s.Fatal("Only one of localFirmwarePath or firmwarePath can be specified")
	}
	if localFirmwarePathVal != "" {
		_, err := os.Stat(localFirmwarePathVal)
		if err != nil {
			s.Fatal("Could not stat specified local firmware path: ", err)
		}
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	flashEC := true
	ecChip, err := h.Servo.GetString(ctx, servo.ECChip)
	if err != nil {
		s.Fatal("Failed to read DUT EC Chip: ", err)
	} else if strings.HasPrefix(ecChip, "it8") { // TODO(b/307797049) Remove this condition once the issue with flash_ec is resolved
		// Flashing EC blocked for ite chips due to b/268108518
		flashEC = false
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
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
	s.Log("Creating tmp directories on dut, servo and testing host")
	tmpDir, err := os.MkdirTemp("", "firmware-UpdateDUTFirmwareServo")
	if err != nil {
		s.Fatal("Failed to create a new directory for the test: ", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	servoTmpDir := tmpServoFirmwareDir
	bashCmd := fmt.Sprintf("test -d %s;echo $?", tmpServoFirmwareDir)
	if out, err := h.ServoProxy.OutputCommand(ctx, false, "bash", "-c", bashCmd); err != nil || string(out) != "0" {
		s.Logf("Error while testing directory exists or directory not found: %s - %q", tmpServoFirmwareDir, err)
		servoTmpDir = alternateServoFirmwareDir
	}

	servoTmpDir = fmt.Sprintf("%s-%s", servoTmpDir, uuid)
	s.Logf("Servo tmp dir: %s", servoTmpDir)
	if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", servoTmpDir); err != nil {
		s.Fatalf("Failed to create temp directory %s on servo for saving existing firmware: %s", servoTmpDir, err)
	}

	// Delete the tmp directory on the servo at the end
	defer func() {
		s.Log("Deleting tmp directory on servo: ", servoTmpDir)
		if err := h.ServoProxy.RunCommand(ctx, false, "rm", "-rf", servoTmpDir); err != nil {
			s.Fatal("Failed to delete temp directory on servo for saving existing firmware: ", err)
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
	if firmwarePathVal != "" || localFirmwarePathVal != "" {
		var ecBinToFlash, monitorBinToFlash, apBinToFlash string
		if firmwarePathVal != "" {
			s.Log("Downloading Firmware to Flash")
			ecBinToFlash, monitorBinToFlash, apBinToFlash = downloadFirmwareFromGCS(ctx, s, tmpDir, firmwarePathVal, fwidModel, flashEC)
		} else {
			ecBinToFlash, monitorBinToFlash, apBinToFlash = untarLocalFirmwareFile(ctx, s, tmpDir, localFirmwarePathVal, fwidModel, flashEC)
		}
		s.Logf("EC Firmware to Flash %s; monitor file to flash %s; AP Firmware to Flash %s", ecBinToFlash, monitorBinToFlash, apBinToFlash)
		fileMap := map[string]string{
			fmt.Sprintf("%s/%s", tmpDir, apBinToFlash): fmt.Sprintf("%s/%s", servoTmpDir, apFirmwareFileToFlash),
		}
		if ecBinToFlash != "" {
			fileMap[fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)] = fmt.Sprintf("%s/%s", servoTmpDir, ecFirmwareFileToFlash)
		}
		if monitorBinToFlash != "" {
			fileMap[fmt.Sprintf("%s/%s", tmpDir, monitorBinToFlash)] = fmt.Sprintf("%s/%s", servoTmpDir, monitorFileToFlash)
		}
		if err := h.ServoProxy.PutFiles(ctx, false, fileMap); err != nil {
			s.Fatal("Failed to copy files to servo host: ", err)
		}
	}
	flashAPFirmware(ctx, s, h, servoTmpDir, firmwarePathVal, ecChip, initialROFwid, initialRwFwid)
	flashECFirmware(ctx, s, h, servoTmpDir, tmpDir, firmwarePathVal, ecChip, flashEC)
}

// downloadFirmwareFromGCS reads a file from GCS based on the board, branch and firmware version specified
func downloadFirmwareFromGCS(ctx context.Context, s *testing.State, tmpDir, firmwareFilepath, model string, flashEC bool) (ecBinToFlash, monitorBinToFlash, apBinToFlash string) {
	// Download the latest shipped firmware.
	if err := fwUtils.DownloadFirmwareFile(ctx, s, tmpDir, firmwareFilepath); err != nil {
		s.Fatal("Failed while downloading file: ", err)
	}
	// Untar the binary file with respect to the model name found in 'crossystem fwid'.
	apBinToFlash, _, err := fwUtils.UntarUnknownFileName(ctx, tmpDir, model, fwUtils.APFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", fwUtils.APFirmware, err)
	}
	if !flashEC {
		return "", "", apBinToFlash
	}
	ecBinToFlash, monitorBinToFlash, err = fwUtils.UntarUnknownFileName(ctx, tmpDir, model, fwUtils.ECFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", fwUtils.ECFirmware, err)
	}
	return ecBinToFlash, monitorBinToFlash, apBinToFlash
}

// untarLocalFirmwareFile untars the provided local firmware file to extract AP and EC images
func untarLocalFirmwareFile(ctx context.Context, s *testing.State, tmpDir, firmwareFilepath, model string, flashEC bool) (ecBinToFlash, monitorBinToFlash, apBinToFlash string) {
	// Copy the fw file to tmp directory.
	dst, err := os.Create(tmpDir + "/" + fwUtils.FirmwareFileName)
	if err != nil {
		s.Fatalf("Failed to open tmp file %q: %s", tmpDir+"/"+fwUtils.FirmwareFileName, err)
	}
	// Close file on exit
	defer func() error {
		if err := dst.Close(); err != nil {
			s.Fatalf("Failed to close tmp file %q: %s", tmpDir+"/"+fwUtils.FirmwareFileName, err)
		}
		return nil
	}()
	src, err := os.Open(firmwareFilepath)
	if err != nil {
		s.Fatalf("Failed to open local firmware file %s for copying to tmp directory: %s", firmwareFilepath, err)
	}
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		s.Fatal("Failed to copy firmware file to tmp location")
	}

	// Untar the binary file with respect to the model name.
	apBinToFlash, _, err = fwUtils.UntarUnknownFileName(ctx, tmpDir, model, fwUtils.APFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", fwUtils.APFirmware, err)
	}
	if !flashEC {
		return "", "", apBinToFlash
	}
	ecBinToFlash, monitorBinToFlash, err = fwUtils.UntarUnknownFileName(ctx, tmpDir, model, fwUtils.ECFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", fwUtils.ECFirmware, err)
	}
	return ecBinToFlash, monitorBinToFlash, apBinToFlash
}

// flashECFirmware flashes the provided EC firmware on the DUT and restores the original EC firmware in the end.
func flashECFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, localTmpDir, firmwarePathVal, ecChip string, flashEC bool) {
	if !flashEC {
		return
	}
	h.DisconnectDUT(ctx)
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Can't restore firmware, DUT is off: ", err)
	}
	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	// Create temp dir on the DUT
	uuid, _ := uuid.NewRandom()
	dutTmpDir := fmt.Sprintf("%s-%s", tmpDutFirmwareDir, uuid)
	if err := dutfs.NewClient(h.RPCClient.Conn).MkDir(ctx, dutTmpDir, 0644); err != nil {
		s.Fatalf("Failed to create temp directory on dut %s for saving existing firmware: %s", dutTmpDir, err)
	}
	defer func() {
		s.Log("Deleting tmp directory on dut: ", dutTmpDir)
		if err := h.RequireRPCClient(ctx); err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		if err := dutfs.NewClient(h.RPCClient.Conn).RemoveAll(ctx, dutTmpDir); err != nil {
			s.Fatal("Failed to delete temp directory on dut for saving existing firmware: ", err)
		}
	}()
	// Backup EC Firmware
	s.Log("Backing up current EC_RW")
	backupRW, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    pb.ImageSection_ECRWImageSection,
		Programmer: pb.Programmer_ECProgrammer,
		Path:       dutTmpDir,
	})
	if err != nil {
		s.Fatal("Failed to backup EC RW firmware: ", err)
	}
	s.Log("Backing up current EC_RO")
	backupRO, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    pb.ImageSection_ECROImageSection,
		Programmer: pb.Programmer_ECProgrammer,
		Path:       dutTmpDir,
	})
	if err != nil {
		s.Fatal("Failed to backup EC RO firmware: ", err)
	}
	s.Log("Completed backup of existing EC fw")
	// Check that the DUT has initial fw in the end
	defer func() {
		h.DisconnectDUT(ctx)
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Can't restore firmware, DUT is off: ", err)
		}
		if err := h.RequireRPCClient(ctx); err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}
		s.Log("Flashing DUT with backup EC RO firmware file: ", backupRO)
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, backupRO); err != nil {
			s.Fatal("Failed to restore EC firmware: ", err)
		}
		s.Log("Flashing DUT with backup EC RW firmware file: ", backupRW)
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, backupRW); err != nil {
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
	// Flash EC
	if firmwarePathVal == "" {
		return
	}
	s.Log("Flashing DUT EC with downloaded firmware file")
	if ecChip == "stm32" {
		if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, ecFirmwareFileToFlash), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--bitbang_rate=57600", "--verify", "--verbose"); err != nil {
			s.Fatal("Failed to flash EC firmware bin file: ", err)
		}
	} else if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, ecFirmwareFileToFlash), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--verify", "--verbose"); err != nil {
		s.Fatal("Failed to flash EC firmware bin file: ", err)
	}
	s.Log("Completed flashing of downloaded ec fw")
}

// flashAPFirmware flashes the provided AP firmware on the DUT and restores the original AP firmware in the end.
func flashAPFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, firmwarePathVal, ecChip, initialROFwid, initialRwFwid string) {
	s.Log("Backing up AP firmware")
	if err := h.ServoProxy.RunCommand(ctx, false, "futility", "read", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile)); err != nil {
		s.Fatal("Failed to read fw using futility: ", err)
	}
	s.Log("Completed backup of existing AP fw")
	// Check that the DUT has initial fw in the end
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
		if err := fwUtils.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
			s.Fatal("Failed while verifying firmware IDs after flashing at the end of test: ", err)
		}
	}()
	if firmwarePathVal == "" {
		return
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
	if err := fwUtils.VerifyFwIDs(ctx, h, firmwarePathVal, firmwarePathVal); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}
