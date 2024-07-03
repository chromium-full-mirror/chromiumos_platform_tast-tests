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
	tmpFirmwareDir     = "/var/tmp/tmp"
	alternateTmpFwDir  = "/tmp/tmp"
	backupFirmwareFile = "backupfw.bin"
	imageGCSBucket     = "chromeos-image-archive"
	defaultTarSuffix   = ".tar.bz2"
	defaultTarballName = "firmware_from_source" + defaultTarSuffix
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

// UpdateDutFirmware flashes the AP and EC firmware if firmware file is specified
// otherwise reads the current AP firmware and flashes it back on the DUT
// Firmware can be specified using the vars "firmware.firmwarePath" for a GCS firmware file path or
// "firmware.localFirmwarePath" for a local firmware file location
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
		s.Log("Path to local firmware file : ", localFirmwarePathVal)
	}
	if firmwarePathVal != "" {
		// Adding default suffix and prefix to GCS firmware file path if needed.
		downloadFilename := firmwarePathVal
		if !strings.HasPrefix(firmwarePathVal, imageGCSBucket) {
			downloadFilename = fmt.Sprintf("%s/%s", imageGCSBucket, firmwarePathVal)
		}
		if !strings.HasSuffix(firmwarePathVal, defaultTarSuffix) {
			downloadFilename = fmt.Sprintf("%s/%s", downloadFilename, defaultTarballName)
		}
		firmwarePathVal = downloadFilename
		s.Log("GCS path to download firmware files : ", firmwarePathVal)
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	ecChip, err := h.Servo.GetString(ctx, servo.ECChip)
	if err != nil {
		s.Fatal("Failed to read DUT EC Chip: ", err)
	}
	s.Log("DUT EC Chip: ", ecChip)

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

	dutTmpDir := fmt.Sprintf("%s-%s", tmpFirmwareDir, uuid)
	servoTmpDir := dutTmpDir
	// check whether the user account running the test has write permission for the tmp dir,
	// if no then check the write permission for the alternate tmp dir. If both don't have permission fail the test.
	if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", servoTmpDir); err != nil {
		s.Logf("Failed to create temp directory %s on servo for saving existing firmware: %s", servoTmpDir, err)
		servoTmpDir = fmt.Sprintf("%s-%s", alternateTmpFwDir, uuid)
		if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", servoTmpDir); err != nil {
			s.Fatalf("Failed to create aletrnate temp directory %s on servo for saving existing firmware: %s", servoTmpDir, err)
		}
	}
	s.Logf("Servo tmp dir: %s", servoTmpDir)

	// Delete the tmp directory on the servo at the end
	defer func() {
		s.Log("Deleting tmp directory on servo: ", servoTmpDir)
		if err := h.ServoProxy.RunCommand(ctx, true, "rm", "-rf", servoTmpDir); err != nil {
			s.Fatal("Failed to delete temp directory on servo for saving existing firmware: ", err)
		}
	}()

	fs := dutfs.NewClient(h.RPCClient.Conn)
	if err := fs.MkDir(ctx, dutTmpDir, 0644); err != nil {
		s.Fatalf("Failed to create temp directory on dut %s for saving existing firmware: %s", dutTmpDir, err)
	}
	s.Log("DUT tmp Directory: ", dutTmpDir)
	defer func() {
		s.Log("Deleting tmp directory on dut: ", dutTmpDir)
		h.CloseRPCConnection(ctx)
		if err := h.RequireRPCClient(ctx); err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		if err := dutfs.NewClient(h.RPCClient.Conn).RemoveAll(ctx, dutTmpDir); err != nil {
			s.Fatal("Failed to delete temp directory on dut for saving existing firmware: ", err)
		}
	}()

	// Get the initial fwid from 'crossystem fwid'.
	initialRwFwid, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		s.Fatal("Failed to get crossystem fwid: ", err)
	}
	re := regexp.MustCompile(`Google_([a-z-A-Z_]*)\.(\d*\.\d*.\d*)`)
	match := re.FindStringSubmatch(initialRwFwid)
	if len(match) != 3 {
		s.Fatalf("Unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, initialRwFwid)
	}
	fwidModel := strings.ToLower(match[1])
	initialRwFwid = match[2]
	s.Logf("FWID Model : %s", fwidModel)

	// Get the RO firmware version ID available on the DUT.
	initialROFwid, err := firmware.GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		s.Fatal("Failed to get AP RO ID: ", err)
	}

	var ecBinToFlash, monitorBinToFlash, apBinToFlash string
	if firmwarePathVal != "" || localFirmwarePathVal != "" {
		if firmwarePathVal != "" {
			s.Log("Downloading Firmware to Flash")
			firmwareFilesToFlash, err := firmware.DownloadRequiredFirmwareFiles(ctx, h, s.CloudStorage(), firmwarePathVal, servoTmpDir, fwidModel)
			if err != nil {
				s.Fatal("Error while downloading firmware files: ", err)
			}

			apBinToFlash = firmwareFilesToFlash.APFirmwareFile
			ecBinToFlash = firmwareFilesToFlash.ECFirmwareFile
			if apBinToFlash == "" || ecBinToFlash == "" {
				s.Fatalf("Failed to download required firmware files; APBinToFlash: %s; ECBinToFlash: %s ", apBinToFlash, ecBinToFlash)
			}

			// copy firmware files to the local host as they need to be copied to the DUT for DUT firmware flashing test
			if err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", servoTmpDir, firmware.APFirmwareFileToFlash), fmt.Sprintf("%s/%s", tmpDir, apBinToFlash)); err != nil {
				s.Fatal("Failed to copy AP firmware file from servo host: ", err)
			}
			if err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", servoTmpDir, firmware.ECFirmwareFileToFlash), fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)); err != nil {
				s.Fatal("Failed to copy EC firmware file from servo host: ", err)
			}

			if firmwareFilesToFlash.MonitorFile != "" {
				monitorBinToFlash = firmwareFilesToFlash.MonitorFile
				if err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", servoTmpDir, firmware.MonitorFileToFlash), fmt.Sprintf("%s/%s", tmpDir, monitorBinToFlash)); err != nil {
					s.Fatal("Failed to copy EC monitor firmware file from servo host: ", err)
				}
			}
		} else {
			ecBinToFlash, monitorBinToFlash, apBinToFlash = untarLocalFirmwareFile(ctx, s, tmpDir, localFirmwarePathVal, fwidModel)
			// copy firmware files to the labstation as they are needed for servo firmware flashing test
			fileMap := map[string]string{
				fmt.Sprintf("%s/%s", tmpDir, apBinToFlash): fmt.Sprintf("%s/%s", servoTmpDir, firmware.APFirmwareFileToFlash),
			}
			if ecBinToFlash != "" {
				fileMap[fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)] = fmt.Sprintf("%s/%s", servoTmpDir, firmware.ECFirmwareFileToFlash)
			}
			if monitorBinToFlash != "" {
				fileMap[fmt.Sprintf("%s/%s", tmpDir, monitorBinToFlash)] = fmt.Sprintf("%s/%s", servoTmpDir, firmware.MonitorFileToFlash)
			}
			if err := h.ServoProxy.PutFiles(ctx, false, fileMap); err != nil {
				s.Fatal("Failed to copy files to servo host: ", err)
			}
		}
		s.Logf("EC Firmware to Flash %s; monitor file to flash %s; AP Firmware to Flash %s", ecBinToFlash, monitorBinToFlash, apBinToFlash)
		dutFileMap := map[string]string{}
		if ecBinToFlash != "" {
			dutFileMap[fmt.Sprintf("%s/%s", tmpDir, ecBinToFlash)] = fmt.Sprintf("%s/%s", dutTmpDir, firmware.ECFirmwareFileToFlash)
		}
		if monitorBinToFlash != "" {
			dutFileMap[fmt.Sprintf("%s/%s", tmpDir, monitorBinToFlash)] = fmt.Sprintf("%s/%s", dutTmpDir, firmware.MonitorFileToFlash)
		}

		s.Log("Copying EC firmware files to dut")
		if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), dutFileMap, linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to copy files to dut: ", err)
		}
		s.Logf("Files under %s: before FW flashing", dutTmpDir)
		statFirmwareFilesOnDUT(ctx, s, h, dutTmpDir)
		flashECFirmwareFromDut(ctx, s, h, dutTmpDir, tmpDir, ecBinToFlash, monitorBinToFlash, ecChip)
		flashECFirmware(ctx, s, h, servoTmpDir, tmpDir, ecChip)
	}

	// AP firmware file is copied to the DUT after EC flashing to handle a corner case for some models
	// where all the firmware files in dut tmp directory become empty after EC firmware flashing.
	dutFileMap := map[string]string{
		fmt.Sprintf("%s/%s", tmpDir, apBinToFlash): fmt.Sprintf("%s/%s", dutTmpDir, firmware.APFirmwareFileToFlash),
	}
	s.Log("Copying AP firmware file to dut")
	if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), dutFileMap, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy files to dut: ", err)
	}
	s.Logf("Files under %s: before AP flashing", dutTmpDir)
	statFirmwareFilesOnDUT(ctx, s, h, dutTmpDir)
	flashAPFirmwareFromDut(ctx, s, h, dutTmpDir, tmpDir, firmwarePathVal, localFirmwarePathVal, initialROFwid, initialRwFwid)
	flashAPFirmware(ctx, s, h, servoTmpDir, firmwarePathVal, localFirmwarePathVal, ecChip, initialROFwid, initialRwFwid)
}

// untarLocalFirmwareFile untars the provided local firmware file to extract AP and EC images
func untarLocalFirmwareFile(ctx context.Context, s *testing.State, tmpDir, firmwareFilepath, model string) (ecBinToFlash, monitorBinToFlash, apBinToFlash string) {
	// Copy the fw file to tmp directory.
	dst, err := os.Create(tmpDir + "/" + firmware.FirmwareFileName)
	s.Logf("Firmware File Path: %s; Tmp File Path: %s", firmwareFilepath, tmpDir)
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
		s.Fatalf("Failed to copy firmware file to tmp location: %s", err)
	}

	// Untar the binary file with respect to the model name.
	apBinToFlash, _, err = firmware.UntarUnknownFileName(ctx, tmpDir, model, firmware.APFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", firmware.APFirmware, err)
	}
	ecBinToFlash, monitorBinToFlash, err = firmware.UntarUnknownFileName(ctx, tmpDir, model, firmware.ECFirmware)
	if err != nil {
		s.Fatalf("Failed to untar file for %s: %s", firmware.ECFirmware, err)
	}
	return ecBinToFlash, monitorBinToFlash, apBinToFlash
}

// flashECFirmware flashes the provided EC firmware on the DUT and restores the original EC firmware in the end.
func flashECFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, localTmpDir, ecChip string) {
	backupECFirmware(ctx, s, h, servoTmpDir, ecChip)

	// Check that the DUT has initial fw in the end
	defer func() {
		h.DisconnectDUT(ctx)
		runECFirmwareFlashServo(ctx, s, h, servoTmpDir, ecChip, backupFirmwareFile)
	}()

	// Flash EC
	s.Log("Flashing DUT EC with downloaded firmware file")
	runECFirmwareFlashServo(ctx, s, h, servoTmpDir, ecChip, firmware.ECFirmwareFileToFlash)
	s.Log("Completed flashing of downloaded ec fw")
}

// flashAPFirmware flashes the provided AP firmware on the DUT and restores the original AP firmware in the end.
func flashAPFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, firmwarePathVal, localFirmwarePathVal, ecChip, initialROFwid, initialRwFwid string) {
	s.Log("Backing up AP firmware")
	if err := h.ServoProxy.RunCommand(ctx, true, "futility", "read", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile)); err != nil {
		s.Fatal("Failed to read fw using futility: ", err)
	}
	s.Log("Completed backup of existing AP fw")
	// Check that the DUT has initial fw in the end
	defer func() {
		s.Log("Flashing DUT with backup AP firmware file")
		if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "-i", fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile), fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), "--gbb_flags=0x18"); err != nil {
			s.Log("Failed to flash DUT backup FW bin file: ", err)
		} else {
			s.Log("Completed flashing of backup AP fw")
		}
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
		}

		// Verify RO/RW firmware versions are the prior ones after flashing.
		// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
		if err := firmware.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
			s.Log("Failed while verifying firmware IDs after flashing backup fw at the end of test: ", err)
		}
	}()
	if firmwarePathVal == "" && localFirmwarePathVal == "" {
		return
	}

	s.Log("Flashing DUT AP with downloaded firmware file")
	if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "-i", fmt.Sprintf("%s/%s", servoTmpDir, firmware.APFirmwareFileToFlash), fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), "--gbb_flags=0x18"); err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err)
	}
	s.Log("Completed flashing of downloaded fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// To verify firmware versions we need the filename to be in a certain format which
	// locally downloaded firmware file may not be in
	// so only verying versions for firmware downloaded from GCS
	if firmwarePathVal == "" {
		return
	}

	// Verify RO/RW firmware versions are the downloaded firmware versions after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err := firmware.VerifyFwIDs(ctx, h, firmwarePathVal, firmwarePathVal); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}

// flashAPFirmwareFromDut flashes the provided AP firmware on the DUT and restores the original AP firmware in the end.
func flashAPFirmwareFromDut(ctx context.Context, s *testing.State, h *firmware.Helper, dutTmpDir, localTmpDir, firmwarePathVal, localFirmwarePathVal, initialROFwid, initialRwFwid string) {
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
		if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(),
			map[string]string{fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile): fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile)},
			linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to copy files to dut: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-i", fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile)).Run(); err != nil {
			s.Log("Failed to flash backup firmware bin file: ", err)
		} else {
			s.Log("Completed flashing of backup AP fw")
		}

		if err := safeRebootDut(ctx, h); err != nil {
			s.Fatal("Failed to reboot DUT after flashing: ", err)
		}

		// Verify RO/RW firmware versions are the prior ones after flashing.
		// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
		if err := firmware.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
			s.Log("Failed while verifying firmware IDs after flashing backup fw at the end of test: ", err)
		}
	}()
	if firmwarePathVal == "" && localFirmwarePathVal == "" {
		return
	}

	s.Log("Flashing DUT AP with downloaded firmware file")
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-i", fmt.Sprintf("%s/%s", dutTmpDir, firmware.APFirmwareFileToFlash)).Run(); err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err)
	}
	s.Log("Completed flashing of downloaded fw")
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// To verify firmware versions we need the filename to be in a certain format which
	// locally downloaded firmware file may not be in
	// so only verying versions for firmware downloaded from GCS
	if firmwarePathVal == "" {
		return
	}

	// Verify RO/RW firmware versions are the downloaded firmware versions after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err := firmware.VerifyFwIDs(ctx, h, firmwarePathVal, firmwarePathVal); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}

// flashECFirmwareFromDut flashes the provided EC firmware on the DUT and restores the original EC firmware in the end.
func flashECFirmwareFromDut(ctx context.Context, s *testing.State, h *firmware.Helper, tmpFwDir, localTmpDir, ecBinToFlash, monitorBinToFlash, ecChip string) {
	s.Log("Backing up EC firmware")
	backupECFirmware(ctx, s, h, tmpFwDir, ecChip)
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
		if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile): fmt.Sprintf("%s/%s", tmpFwDir, backupFirmwareFile)}, linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to copy files to dut: ", err)
		}
		runECFirmwareFlashDut(ctx, s, h, tmpFwDir, backupFirmwareFile, true)
		s.Log("Completed flashing of backup EC fw")
	}()

	s.Log("Flashing DUT EC with downloaded firmware file")
	if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{fmt.Sprintf("%s/%s", localTmpDir, ecBinToFlash): fmt.Sprintf("%s/%s", tmpFwDir, firmware.ECFirmwareFileToFlash)}, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy files to dut: ", err)
	}
	runECFirmwareFlashDut(ctx, s, h, tmpFwDir, firmware.ECFirmwareFileToFlash, false)
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
func backupECFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, ecChip string) {
	h.DisconnectDUT(ctx)
	flashCmd := fmt.Sprintf("cd %s&&flash_ec --port=%d --read=%s/%s", servoTmpDir, h.ServoProxy.GetPort(), servoTmpDir, backupFirmwareFile)
	if strings.HasPrefix(ecChip, "it8") {
		flashCmd += " --nouse_i2c_pseudo"
	}
	if err := h.ServoProxy.RunCommand(ctx, true, "bash", "-c", flashCmd); err != nil {
		s.Fatal("Failed to backup EC firmware: ", err)
	}
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after backup EC: ", err)
	}
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Can't restore firmware, DUT is off: ", err)
	}
}

// runECFirmwareFlashServo runs EC firmware flashing from the Servo
func runECFirmwareFlashServo(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, ecChip, image string) {
	flashECArgs := []string{fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", servoTmpDir, image), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--verify", "--verbose"}
	if ecChip == "stm32" {
		flashECArgs = append(flashECArgs, "--bitbang_rate=57600")
	}
	if strings.HasPrefix(ecChip, "it8") {
		flashECArgs = append(flashECArgs, "--nouse_i2c_pseudo")
	}
	if err := h.ServoProxy.RunCommand(ctx, true, "flash_ec", flashECArgs...); err != nil {
		s.Fatal("Failed to flash EC firmware bin file: ", err)
	}
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Can't restore firmware, DUT is off: ", err)
	}
}

// runECFirmwareFlashDut runs EC firmware flashing from the DUT
func runECFirmwareFlashDut(ctx context.Context, s *testing.State, h *firmware.Helper, dutTmpDir, image string, allowFlashFailure bool) {
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "--ec_image", fmt.Sprintf("%s/%s", dutTmpDir, image)).Run(); err != nil {
		if !allowFlashFailure {
			s.Fatal("Failed to flash firmware bin file: ", err)
		}
		s.Log("Failed to flash firmware bin file: ", err)
	}
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}
}

// statFirmwareFilesOnDUT stats the files in the temporary fw dir on the DUT
func statFirmwareFilesOnDUT(ctx context.Context, s *testing.State, h *firmware.Helper, tmpFwDir string) {
	h.CloseRPCConnection(ctx)
	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	fs := dutfs.NewClient(h.RPCClient.Conn)

	fis, err := fs.ReadDir(ctx, tmpFwDir)
	if err != nil {
		s.Fatalf("Failed to list files at %s: %v", tmpFwDir, err)
	}

	for _, fi := range fis {
		s.Logf("Firmware file Name: %s; file size: %d", fi.Name(), fi.Size())
	}
}
