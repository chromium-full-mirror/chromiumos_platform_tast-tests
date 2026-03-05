// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	errors "go.chromium.org/tast/core/errors"
	linuxssh "go.chromium.org/tast/core/ssh/linuxssh"
	testing "go.chromium.org/tast/core/testing"
)

const (
	tmpFirmwareDir     = "/var/tmp/tmp"
	alternateTmpFwDir  = "/tmp/tmp"
	backupFirmwareFile = "backupfw.bin"
	imageGCSBucket     = "chromeos-image-archive"
	defaultTarSuffix   = ".tar.bz2"
	defaultTarballName = "firmware_from_source" + defaultTarSuffix
)

// Versions is the internal object of FW manifest
type versions struct {
	Ro string `json:"ro"`
	Rw string `json:"rw"`
}

// Keys is the internal object of FW manifest
type keys struct {
	Root     string `json:"root"`
	Recovery string `json:"recovery"`
}

// Host is the internal object of FW manifest
type host struct {
	Versions versions `json:"versions"`
	Keys     keys     `json:"keys"`
	Image    string   `json:"image"`
}

// Default is the internal object of FW manifest
type defaults struct {
	Host host `json:"host"`
}

// FWManifest represents the structure of the FW manifest read from FW tarball
type fWManifest struct {
	Default defaults `json:"default"`
}

// fwIDs represents a structure for initial and target FW IDs
type fwIDs struct {
	initialROFwid string
	initialRWFwid string
	targetROFwid  string
	targetRWFwid  string
}

var (
	// skipFWRestore indicates whether to skip restoring FW in the end
	skipFWRestore = testing.RegisterVarString(
		"labqual.skipFwRestore",
		"false",
		"A booelan variable to store information about FW should be restored to original at the end of the test; Use 'true' or 'false'")
)

func shouldRestoreFW() bool {
	return skipFWRestore.Value() != "true"
}

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateDutFirmware,
		Desc: "Update AP and EC firmware from Servo",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_stable"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService", "dutfs.ServiceName"},
		Fixture:      fixture.NormalMode,
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
	if firmwarePathVal == "" && localFirmwarePathVal == "" {
		s.Fatal("Atleast one of localFirmwarePath or firmwarePath should be specified")
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
		if !strings.HasPrefix(firmwarePathVal, "gs://") && !strings.HasPrefix(firmwarePathVal, imageGCSBucket) {
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

	// Skip checking for EC crashes in this test because some of the firmware
	// flashed during this test can be flaky so crashes are expected.
	h.CheckECCrash = false

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
			if err := h.Servo.SetString(ctx, servo.GSCTestlab, servo.Open); err != nil {
				s.Fatal("Failed to unlock CCD: ", err)
			}
		}
	}

	s.Log("Disabling hardware write protect")
	err = h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff)
	if err != nil {
		s.Fatal("Failed to disable hardware write protect: ", err)
	}
	s.Log("Disabling software write protect")
	out, err := h.ServoProxy.OutputCommand(ctx, true, "futility", "flash", "--wp-disable", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()))
	if err != nil {
		s.Fatalf("Software write protect disable failed: %q, Output: %s", err, string(out))
	}
	s.Logf("Disabling software write protect completed, command output: %s", string(out))

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
	h.CloseRPCConnection(ctx)
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
	re := regexp.MustCompile(`Google_(\w*)\.(\d*\.\d*.\d*)`)
	match := re.FindStringSubmatch(initialRwFwid)
	if len(match) != 3 {
		s.Fatalf("Unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, initialRwFwid)
	}
	fwidModel := strings.ToLower(match[1])
	initialRwFwid = match[2]
	s.Logf("FWID Model : %s", fwidModel)
	fwTargets, _ := firmware.ReadFirmwareTargets(ctx, s.DUT().Conn(), h.Model, fwidModel)
	s.Logf("Found AP Target: %s and EC Target: %s", fwTargets.APTarget, fwTargets.ECTarget)

	// Get the RO firmware version ID available on the DUT.
	initialROFwid, err := h.Reporter.GetFWVersion(ctx, reporters.CrossystemParamRoFwid)
	if err != nil {
		s.Fatal("Failed to get AP RO ID: ", err)
	}

	var ecBinToFlash, monitorBinToFlash, apBinToFlash string
	if firmwarePathVal != "" || localFirmwarePathVal != "" {
		if firmwarePathVal != "" {
			s.Log("Downloading Firmware to Flash")
			firmwareFilesToFlash, err := firmware.DownloadFirmwareFiles(ctx, s.CloudStorage(), h, servoTmpDir, firmwarePathVal, "", fwTargets)
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
	apFirmwareFile := fmt.Sprintf("%s/%s", servoTmpDir, firmware.APFirmwareFileToFlash)
	versions := getFWVersionsFromManifest(ctx, s, h, firmwarePathVal, apFirmwareFile)
	fwids := fwIDs{
		initialROFwid: initialROFwid,
		initialRWFwid: initialRwFwid,
		targetROFwid:  versions.Ro,
		targetRWFwid:  versions.Rw,
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
	flashAPFirmwareFromDut(ctx, s, h, dutTmpDir, tmpDir, firmwarePathVal, localFirmwarePathVal, fwids)
	flashAPFirmware(ctx, s, h, servoTmpDir, firmwarePathVal, localFirmwarePathVal, ecChip, fwids)
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
	if shouldRestoreFW() {
		s.Log("Backing up EC firmware")
		isBackupEC, err := backupECFirmware(ctx, s, h, servoTmpDir, ecChip)
		if err != nil {
			s.Log("Error while taking a backup of existing EC fw: ", err)
		}
		s.Log("Completed backup of existing EC fw")

		// Check that the DUT has initial fw in the end
		defer func() {
			if isBackupEC {
				h.DisconnectDUT(ctx)
				runECFirmwareFlashServo(ctx, s, h, servoTmpDir, ecChip, backupFirmwareFile)

			}
		}()
	}

	// Flash EC
	s.Log("Flashing DUT EC with downloaded firmware file")
	runECFirmwareFlashServo(ctx, s, h, servoTmpDir, ecChip, firmware.ECFirmwareFileToFlash)
	s.Log("Completed flashing of downloaded ec fw")
}

// flashAPFirmware flashes the provided AP firmware on the DUT and restores the original AP firmware in the end.
func flashAPFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, firmwarePathVal, localFirmwarePathVal, ecChip string, fwids fwIDs) {
	futilityInstance, _ := futility.NewRemoteBuilder(h.ServoProxy).Build()
	if shouldRestoreFW() {
		s.Log("Backing up AP firmware")
		isBackupAP := false
		backupFirmwareFile := fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile)
		readOpts := futility.NewReadAPOptions(backupFirmwareFile)
		log, err := futilityInstance.ReadAP(ctx, readOpts)
		if err != nil {
			s.Fatalf("Failed to read existing AP firmware: %v, got futility log: %s", err, string(log))
		} else {
			isBackupAP = true
			s.Logf("Completed backup of existing AP fw, command output: %s", log)
			if err := h.EnsureDUTBooted(ctx); err != nil {
				s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
			}
		}

		// Check that the DUT has initial fw in the end
		defer func() {
			if isBackupAP {
				s.Log("Flashing DUT with backup AP firmware file")
				// For dm-default-key layouts, the dev image preservation requires an extra preservation step.
				// The binary will return success on all other layouts.
				if h.DUT.Connected(ctx) {
					if err := h.DUT.Conn().CommandContext(ctx, "/usr/local/bin/preserve_dev_image").Run(); err != nil {
						s.Log("Failed preserving dev image: ", err)
					}
				}

				flashOpts := futility.NewUpdateOptions(backupFirmwareFile).
					WithMode(futility.UpdateModeRecovery).
					WithGBBFlags(24)
				out, err := futilityInstance.Update(ctx, flashOpts)
				if err != nil {
					s.Logf("Failed to flash firmware bin file: %s, Output:%s", err, string(out))
				} else {
					s.Logf("Completed flashing of backup AP fw, command output: %s", string(out))
				}
				if err := safeRebootDut(ctx, h); err != nil {
					s.Fatal("Failed to reboot DUT after flashing: ", err)
				}

				// Verify RO/RW firmware versions are the prior ones after flashing.
				// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
				if err := firmware.VerifyFwIDs(ctx, h, fwids.initialROFwid, fwids.initialRWFwid); err != nil {
					s.Log("Failed while verifying firmware IDs after flashing backup fw at the end of test: ", err)
				}
			}
		}()
	}
	if firmwarePathVal == "" && localFirmwarePathVal == "" {
		return
	}

	// For dm-default-key layouts, the dev image preservation requires an extra preservation step.
	// The binary will return success on all other layouts.
	if err := h.DUT.Conn().CommandContext(ctx, "/usr/local/bin/preserve_dev_image").Run(); err != nil {
		s.Fatal("Failed preserving dev image: ", err)
	}

	s.Log("Flashing DUT AP with downloaded firmware file")
	apFirmwareFile := fmt.Sprintf("%s/%s", servoTmpDir, firmware.APFirmwareFileToFlash)
	flashOpts := futility.NewUpdateOptions(apFirmwareFile).
		WithMode(futility.UpdateModeRecovery).
		WithGBBFlags(24)
	out, err := futilityInstance.Update(ctx, flashOpts)
	if err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err, "\nOutput:\n", string(out))
	}
	s.Logf("Completed flashing of downloaded fw, command output: %s", string(out))
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}

	// To verify firmware versions we need the filename to be in a certain format which
	// locally downloaded firmware file may not be in
	// so only verying versions for firmware downloaded from GCS
	if firmwarePathVal == "" {
		return
	}

	// Verify RO/RW firmware versions are the downloaded firmware versions after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err := firmware.VerifyFwIDs(ctx, h, fwids.targetROFwid, fwids.targetRWFwid); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}

// flashAPFirmwareFromDut flashes the provided AP firmware on the DUT and restores the original AP firmware in the end.
func flashAPFirmwareFromDut(ctx context.Context, s *testing.State, h *firmware.Helper, dutTmpDir, localTmpDir, firmwarePathVal, localFirmwarePathVal string, fwids fwIDs) {
	futilityInstance, _ := futility.NewLocalBuilder(h.DUT).Build()
	if shouldRestoreFW() {
		s.Log("Backing up AP firmware")
		isBackupAP := true
		readOpts := futility.NewReadAPOptions(fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile))
		log, err := futilityInstance.ReadAP(ctx, readOpts)
		if err != nil {
			isBackupAP = false
			s.Logf("Failed to read existing AP firmware: %v, got futility log: %s", err, string(log))
		}
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
		}
		if isBackupAP {
			s.Logf("Completed backup of existing AP fw, command output: %s", log)

			// Copy backup file from dut to host
			err = linuxssh.GetFile(ctx, s.DUT().Conn(), fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile), fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile), linuxssh.PreserveSymlinks)
			if err != nil {
				s.Fatal("Failed to copy file from DUT to Host: ", err)
			}
		}

		// Restore the initial fw to the DUT in the end
		defer func() {
			if isBackupAP {
				s.Log("Flashing DUT with backup AP firmware file")
				if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(),
					map[string]string{fmt.Sprintf("%s/%s", localTmpDir, backupFirmwareFile): fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile)},
					linuxssh.PreserveSymlinks); err != nil {
					s.Fatal("Failed to copy files to dut: ", err)
				}
				if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-v", "-i", fmt.Sprintf("%s/%s", dutTmpDir, backupFirmwareFile), "--quirks", "ec_partial_recovery").Run(testexec.DumpLogOnError); err != nil {
					s.Log("Failed to flash backup firmware bin file: ", err)
				} else {
					s.Log("Completed flashing of backup AP fw")
				}

				// For dm-default-key layouts, the dev image preservation requires an extra preservation step.
				// The binary will return success on all other layouts.
				if err := h.DUT.Conn().CommandContext(ctx, "/usr/local/bin/preserve_dev_image").Run(); err != nil {
					s.Log("Failed preserving dev image: ", err)
				}

				if err := safeRebootDut(ctx, h); err != nil {
					s.Fatal("Failed to reboot DUT after flashing: ", err)
				}

				// Verify RO/RW firmware versions are the prior ones after flashing.
				// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
				if err := firmware.VerifyFwIDs(ctx, h, fwids.initialROFwid, fwids.initialRWFwid); err != nil {
					s.Log("Failed while verifying firmware IDs after flashing backup fw at the end of test: ", err)
				}
			}
		}()
	}
	if firmwarePathVal == "" && localFirmwarePathVal == "" {
		return
	}

	s.Log("Flashing DUT AP with downloaded firmware file")
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-v", "-i", fmt.Sprintf("%s/%s", dutTmpDir, firmware.APFirmwareFileToFlash), "--quirks", "ec_partial_recovery").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to flash firmware bin file: ", err)
	}

	// For dm-default-key layouts, the dev image preservation requires an extra preservation step.
	// The binary will return success on all other layouts.
	if err := h.DUT.Conn().CommandContext(ctx, "/usr/local/bin/preserve_dev_image").Run(); err != nil {
		s.Fatal("Failed preserving dev image: ", err)
	}

	s.Log("Completed flashing of downloaded fw")
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}

	// To verify firmware versions we need the filename to be in a certain format which
	// locally downloaded firmware file may not be in
	// so only verying versions for firmware downloaded from GCS
	if firmwarePathVal == "" {
		return
	}

	// Verify RO/RW firmware versions are the downloaded firmware versions after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err := firmware.VerifyFwIDs(ctx, h, fwids.targetROFwid, fwids.targetRWFwid); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", firmwarePathVal, firmwarePathVal, err)
	}
}

// flashECFirmwareFromDut flashes the provided EC firmware on the DUT and restores the original EC firmware in the end.
func flashECFirmwareFromDut(ctx context.Context, s *testing.State, h *firmware.Helper, tmpFwDir, localTmpDir, ecBinToFlash, monitorBinToFlash, ecChip string) {
	if shouldRestoreFW() {
		s.Log("Backing up EC firmware")
		isBackupEC, err := backupECFirmware(ctx, s, h, tmpFwDir, ecChip)
		if err != nil {
			s.Log("Error while taking a backup of existing EC fw: ", err)
		}
		s.Log("Completed backup of existing EC fw")

		// Check that the DUT has initial fw in the end
		defer func() {
			// Copy backup file from servo to host
			if isBackupEC {
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
			}
		}()
	}

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
	if err := h.EnsureDUTBooted(ctx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after reboot")
	}
	return nil
}

// getFWVersionsFromManifest reads the firmware versions from the firmware manifest file
func getFWVersionsFromManifest(ctx context.Context, s *testing.State, h *firmware.Helper, firmwarePathVal, apFirmwareFile string) *versions {
	s.Log("Reading manifest from downloaded firmware file")

	// Read the firmware manifest file from the Servo without passing the servo_port flag to futility.
	// Example command: futility update --image=/tmp/image-deku.bin --manifest
	out, err := h.ServoProxy.OutputCommand(ctx, true, "futility", "update", fmt.Sprintf("--image=%s", apFirmwareFile), "--manifest")
	if err != nil {
		s.Fatal("Failed to read manifest file: ", err, "\nOutput:\n", string(out))
	}
	s.Logf("Got manifest file from fw, command output: %s", strings.Join(strings.Fields(string(out)), ""))
	versions, err := parseFwManifest(ctx, s, out)
	if err != nil {
		s.Fatal("Failed to read manifest data: ", err)
	}
	s.Logf("Got RO version from manifest: %s", versions.Ro)
	if err := safeRebootDut(ctx, h); err != nil {
		s.Fatal("Failed to reboot DUT after flashing: ", err)
	}
	if versions.Ro == "" {
		versions.Ro = firmwarePathVal
	}
	if versions.Rw == "" {
		versions.Rw = firmwarePathVal
	}
	return versions
}

// parseFwManifest parses the firmware manifest json blob
func parseFwManifest(ctx context.Context, s *testing.State, manifest []byte) (*versions, error) {
	re := regexp.MustCompile(`.*INFO.*`)
	match := re.Split(string(manifest), 2)

	var root fWManifest
	err := json.Unmarshal([]byte(strings.Join(strings.Fields(string(match[0])), "")), &root)
	if err != nil {
		return nil, err
	}

	s.Logf("Manifest Data: %s", root)
	return &versions{Ro: getFWVersion(ctx, s, root.Default.Host.Versions.Ro), Rw: getFWVersion(ctx, s, root.Default.Host.Versions.Rw)}, nil
}

// getFWVersion accepts Fwid as input, splits the outputs and only returns the version numbers.
func getFWVersion(ctx context.Context, s *testing.State, fwid string) string {
	re := regexp.MustCompile(`Google_([a-z-A-Z_]*)\.(\d*\.\d*.\d*)`)
	match := re.FindStringSubmatch(fwid)
	if len(match) != 3 {
		s.Logf("Unexpected fw id format got: %s", fwid)
		return ""
	}
	fwidModel := strings.ToLower(match[2])
	return fwidModel
}

// backupECFirmware takes a backup of current EC firmware.
func backupECFirmware(ctx context.Context, s *testing.State, h *firmware.Helper, servoTmpDir, ecChip string) (bool, error) {
	h.DisconnectDUT(ctx)
	flashCmd := fmt.Sprintf("cd %s&&flash_ec --verbose --port=%d --read=%s/%s", servoTmpDir, h.ServoProxy.GetPort(), servoTmpDir, backupFirmwareFile)
	if strings.HasPrefix(ecChip, "it8") {
		flashCmd += " --nouse_i2c_pseudo"
	}
	if err := h.ServoProxy.RunCommand(ctx, true, "bash", "-c", flashCmd); err != nil {
		return false, errors.Wrap(err, "failed to backup EC firmware")
	}
	if err := safeRebootDut(ctx, h); err != nil {
		return false, errors.Wrap(err, "failed to reboot DUT after backup EC")
	}
	return true, nil
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
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "-v", "--ec_image", fmt.Sprintf("%s/%s", dutTmpDir, image), "--quirks", "ec_partial_recovery").Run(testexec.DumpLogOnError); err != nil {
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
