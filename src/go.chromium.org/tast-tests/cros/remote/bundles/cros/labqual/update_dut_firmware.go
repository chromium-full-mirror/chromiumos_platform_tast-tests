// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	tmpFirmwareDir        = "/var/tmp"
	backupFirmwareFile    = "backupfw.bin"
	ecFirmwareFileToFlash = "ecFirmwareForTest.bin"
	apFirmwareFileToFlash = "FirmwareForTest.bin"
	monitorFileToFlash    = "npcx_monitor.bin"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateDutFirmware,
		Desc: "Update AP and EC firmware from Servo",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService", "dutfs.ServiceName"},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      90 * time.Minute, // 1hr30min.
	})
}

// UpdateDutFirmware reads the current AP firmware and flashes it back from the servo using futility
func UpdateDutFirmware(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	firmwarePathVal := string(firmware.FirmwarePath.Value())
	localFirmwarePathVal := string(firmware.LocalFirmwarePath.Value())

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

	tmpFwDir := fmt.Sprintf("%s-%s", tmpFirmwareDir, uuid)
	s.Logf("Servo tmp dir: %s", tmpFwDir)
	if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", tmpFwDir); err != nil {
		s.Fatalf("Failed to create temp directory %s on servo for saving existing firmware: %s", tmpFwDir, err)
	}
	// Delete the tmp directory on the servo at the end
	defer func() {
		s.Log("Deleting tmp directory on servo: ", tmpFwDir)
		if err := h.ServoProxy.RunCommand(ctx, false, "rm", "-rf", tmpFwDir); err != nil {
			s.Fatal("Failed to delete temp directory on servo for saving existing firmware: ", err)
		}
	}()

	fs := dutfs.NewClient(h.RPCClient.Conn)
	if err := fs.MkDir(ctx, tmpFwDir, 0644); err != nil {
		s.Fatalf("Failed to create temp directory on dut %s for saving existing firmware: %s", tmpFwDir, err)
	}
	s.Log("DUT tmp Directory: ", tmpFwDir)
	defer func() {
		s.Log("Deleting tmp directory on dut: ", tmpFwDir)
		h.CloseRPCConnection(ctx)
		if err := h.RequireRPCClient(ctx); err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		if err := dutfs.NewClient(h.RPCClient.Conn).RemoveAll(ctx, tmpFwDir); err != nil {
			s.Fatal("Failed to delete temp directory on dut for saving existing firmware: ", err)
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
	initialROFwid, err := firmware.GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		s.Fatal("Failed to get AP RO ID: ", err)
	}
	var ecBinToFlash, monitorBinToFlash, apBinToFlash string
	if firmwarePathVal != "" || localFirmwarePathVal != "" {
		if firmwarePathVal != "" {
			s.Log("Downloading Firmware to Flash")
			ecBinToFlash, monitorBinToFlash, apBinToFlash = downloadFirmwareFromGCS(ctx, s, tmpDir, firmwarePathVal, fwidModel, flashEC)
		} else {
			ecBinToFlash, monitorBinToFlash, apBinToFlash = untarLocalFirmwareFile(ctx, s, tmpDir, localFirmwarePathVal, fwidModel, flashEC)
		}
		s.Logf("EC Firmware to Flash %s; monitor file to flash %s; AP Firmware to Flash %s", ecBinToFlash, monitorBinToFlash, apBinToFlash)
		fileMap := map[string]string{
			fmt.Sprintf("%s/%s", tmpDir, apBinToFlash): fmt.Sprintf("%s/%s", tmpFwDir, apFirmwareFileToFlash),
		}
		dutFileMap := map[string]string{
			fmt.Sprintf("%s/%s", tmpDir, apBinToFlash): fmt.Sprintf("%s/%s", tmpFwDir, apFirmwareFileToFlash),
		}
		if ecBinToFlash != "" {
			fileMap[fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)] = fmt.Sprintf("%s/%s", tmpFwDir, ecFirmwareFileToFlash)
			dutFileMap[fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)] = fmt.Sprintf("%s/%s", tmpFwDir, ecFirmwareFileToFlash)
		}
		if monitorBinToFlash != "" {
			fileMap[fmt.Sprintf("%s/%s", tmpDir, monitorBinToFlash)] = fmt.Sprintf("%s/%s", tmpFwDir, monitorFileToFlash)
			dutFileMap[fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)] = fmt.Sprintf("%s/%s", tmpFwDir, monitorFileToFlash)
		}
		if err := h.ServoProxy.PutFiles(ctx, false, fileMap); err != nil {
			s.Fatal("Failed to copy files to servo host: ", err)
		}
		s.Log("Cpoying files to dut")
		linuxssh.PutFiles(ctx, s.DUT().Conn(), dutFileMap, linuxssh.PreserveSymlinks)
	}
	flashAPFirmwareFromDut(ctx, s, h, tmpFwDir, tmpDir, firmwarePathVal, initialROFwid, initialRwFwid)
	flashAPFirmware(ctx, s, h, tmpFwDir, firmwarePathVal, ecChip, initialROFwid, initialRwFwid)
	flashECFirmware(ctx, s, h, tmpFwDir, tmpDir, firmwarePathVal, ecChip, flashEC)
	flashECFirmwareFromDut(ctx, s, h, tmpFwDir, tmpDir, firmwarePathVal, ecBinToFlash, monitorBinToFlash)
}

// downloadFirmwareFromGCS reads a file from GCS based on the board, branch and firmware version specified
func downloadFirmwareFromGCS(ctx context.Context, s *testing.State, tmpDir, firmwareFilepath, model string, flashEC bool) (ecBinToFlash, monitorBinToFlash, apBinToFlash string) {
	// Download the latest shipped firmware.
	if err := firmware.DownloadFirmwareFile(ctx, s.CloudStorage(), tmpDir, firmwareFilepath); err != nil {
		s.Fatal("Failed while downloading file: ", err)
	}
	// Untar the binary file with respect to the model name found in 'crossystem fwid'.
	apBinToFlash, _, err := firmware.UntarUnknownFileName(ctx, tmpDir, model, firmware.APFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", firmware.APFirmware, err)
	}
	if !flashEC {
		return "", "", apBinToFlash
	}
	ecBinToFlash, monitorBinToFlash, err = firmware.UntarUnknownFileName(ctx, tmpDir, model, firmware.ECFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", firmware.ECFirmware, err)
	}
	return ecBinToFlash, monitorBinToFlash, apBinToFlash
}

// untarLocalFirmwareFile untars the provided local firmware file to extract AP and EC images
func untarLocalFirmwareFile(ctx context.Context, s *testing.State, tmpDir, firmwareFilepath, model string, flashEC bool) (ecBinToFlash, monitorBinToFlash, apBinToFlash string) {
	// Copy the fw file to tmp directory.
	dst, err := os.Create(tmpDir + "/" + firmware.FirmwareFileName)
	if err != nil {
		s.Fatalf("Failed to open tmp file %q: %s", tmpDir+"/"+firmware.FirmwareFileName, err)
	}
	// Close file on exit
	defer func() error {
		if err := dst.Close(); err != nil {
			s.Fatalf("Failed to close tmp file %q: %s", tmpDir+"/"+firmware.FirmwareFileName, err)
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
	apBinToFlash, _, err = firmware.UntarUnknownFileName(ctx, tmpDir, model, firmware.APFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", firmware.APFirmware, err)
	}
	if !flashEC {
		return "", "", apBinToFlash
	}
	ecBinToFlash, monitorBinToFlash, err = firmware.UntarUnknownFileName(ctx, tmpDir, model, firmware.ECFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", firmware.ECFirmware, err)
	}
	return ecBinToFlash, monitorBinToFlash, apBinToFlash
}

// flashECFirmware flashes the provided EC firmware on the DUT and restores the original EC firmware in the end.
func flashECFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, localTmpDir, firmwarePathVal, ecChip string, flashEC bool) {
	if !flashEC || firmwarePathVal == "" {
		return
	}
	backupECFirmware(ctx, s, h, servoTmpDir)

	// Check that the DUT has initial fw in the end
	defer func() {
		h.DisconnectDUT(ctx)
		runECFirmwareFlashServo(ctx, s, h, servoTmpDir, ecChip, backupFirmwareFile)
	}()

	// Flash EC
	s.Log("Flashing DUT EC with downloaded firmware file")
	runECFirmwareFlashServo(ctx, s, h, servoTmpDir, ecChip, ecFirmwareFileToFlash)
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
		if err := firmware.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
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
	if err := firmware.VerifyFwIDs(ctx, h, firmwarePathVal, firmwarePathVal); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}

// flashAPFirmwareFromDut flashes the provided AP firmware on the DUT and restores the original AP firmware in the end.
func flashAPFirmwareFromDut(ctx context.Context, s *testing.State, h *firmware.Helper, dutTmpDir, localTmpDir, firmwarePathVal, initialROFwid, initialRwFwid string) {
	s.Log("Backing up AP firmware")
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "read", fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile)).Run(); err != nil {
		s.Fatal("Failed to read fw using futility: ", err)
	}
	// Copy backup file from dut to host
	err := linuxssh.GetFile(ctx, s.DUT().Conn(), fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile), fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile), linuxssh.PreserveSymlinks)
	if err != nil {
		s.Fatal("Failed to copy file from DUT to Host: ", err)
	}
	s.Log("Completed backup of existing AP fw")

	// Restore the initial fw to the DUT in the end
	defer func() {
		s.Log("Flashing DUT with backup AP firmware file")
		linuxssh.PutFiles(ctx, s.DUT().Conn(),
			map[string]string{fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile): fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile)},
			linuxssh.PreserveSymlinks)
		if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-i", fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile)).Run(); err != nil {
			s.Fatal("Failed to flash firmware bin file: ", err)
		}
		s.Log("Completed flashing of backup AP fw")

		if err := safeRebootDut(ctx, h); err != nil {
			s.Fatal("Failed to reboot DUT after flashing: ", err)
		}

		// Verify RO/RW firmware versions are the prior ones after flashing.
		// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
		if err := firmware.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
			s.Fatal("Failed while verifying firmware IDs after flashing at the end of test: ", err)
		}
	}()
	if firmwarePathVal == "" {
		return
	}

	s.Log("Flashing DUT AP with downloaded firmware file")
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-i", fmt.Sprintf("%s/%s", dutTmpDir, apFirmwareFileToFlash)).Run(); err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err)
	}
	s.Log("Completed flashing of downloaded fw")
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// Verify RO/RW firmware versions are the downloaded firmware versions after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err := firmware.VerifyFwIDs(ctx, h, firmwarePathVal, firmwarePathVal); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}

// flashECFirmwareFromDut flashes the provided EC firmware on the DUT and restores the original EC firmware in the end.
func flashECFirmwareFromDut(ctx context.Context, s *testing.State, h *firmware.Helper, tmpFwDir, localTmpDir, firmwarePathVal, ecBinToFlash, monitorBinToFlash string) {
	if firmwarePathVal == "" {
		return
	}
	s.Log("Backing up EC firmware")
	backupECFirmware(ctx, s, h, tmpFwDir)
	s.Log("Completed backup of existing EC fw")

	// Check that the DUT has initial fw in the end
	defer func() {
		// Copy backup file from servo to host
		err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", tmpFwDir, backupFirmwareFile), fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile))
		if err != nil {
			s.Fatal("Failed to copy file from Servo to Host: ", err)
		}
		s.Log("Flashing DUT with backup EC firmware file")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
		}
		linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile): fmt.Sprintf("%s/%s", tmpFwDir, backupFirmwareFile), fmt.Sprintf("%s/%s", localTmpDir, ecBinToFlash): fmt.Sprintf("%s/%s", tmpFwDir, ecFirmwareFileToFlash)}, linuxssh.PreserveSymlinks)
		// copyFileToDUT(ctx, s, h, fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile), fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile))
		runECFirmwareFlashDut(ctx, s, h, tmpFwDir, backupFirmwareFile)
		s.Log("Completed flashing of backup EC fw")
	}()

	s.Log("Flashing DUT EC with downloaded firmware file")
	linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{fmt.Sprintf("%s/%s", localTmpDir, monitorBinToFlash): fmt.Sprintf("%s/%s", tmpFwDir, monitorFileToFlash), fmt.Sprintf("%s/%s", localTmpDir, ecBinToFlash): fmt.Sprintf("%s/%s", tmpFwDir, ecFirmwareFileToFlash)}, linuxssh.PreserveSymlinks)
	runECFirmwareFlashDut(ctx, s, h, tmpFwDir, ecFirmwareFileToFlash)
	s.Log("Completed flashing of downloaded fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}
}

// safeRebootDut will close RPC connection, reboot DUT and Open a new RPC connection.
func safeRebootDut(ctx context.Context, h *firmware.Helper) error {
	// Close RPC connection before reboot.
	h.CloseRPCConnection(ctx)

	testing.ContextLog(ctx, "Power-cycling DUT with a cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to reboot DUT by servo")
	}

	testing.ContextLog(ctx, "Waiting for DUT to reconnect")
	connectCtx, cancelconnectCtx := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelconnectCtx()

	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}

	// Open RPC connection after reboot.
	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "failed to open RPC client after reboot")
	}
	return nil
}

// backupECFirmware takes a backup of current EC firmware.
func backupECFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir string) {
	h.DisconnectDUT(ctx)
	flashCmd := fmt.Sprintf("cd %s&&flash_ec --port=%d --read=%s/%s", servoTmpDir, h.ServoProxy.GetPort(), servoTmpDir, backupFirmwareFile)
	if err := h.ServoProxy.RunCommand(ctx, false, "bash", "-c", flashCmd); err != nil {
		s.Fatal("Failed to backup EC firmware: ", err)
	}
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after backup EC: ", err)
	}
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Can't restore firmware, DUT is off: ", err)
	}
}

func runECFirmwareFlashServo(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, ecChip, image string) {
	if ecChip == "stm32" {
		if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, image), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--bitbang_rate=57600", "--verify", "--verbose"); err != nil {
			s.Fatal("Failed to flash EC firmware bin file: ", err)
		}
	} else if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, image), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--verify", "--verbose"); err != nil {
		s.Fatal("Failed to flash EC firmware bin file: ", err)
	}
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Can't restore firmware, DUT is off: ", err)
	}
}

func runECFirmwareFlashDut(ctx context.Context, s *testing.State, h *firmware.Helper, dutTmpDir, image string) {
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "--ec_image", fmt.Sprintf("%s/%s", dutTmpDir, image)).Run(); err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err)
	}
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}
}
