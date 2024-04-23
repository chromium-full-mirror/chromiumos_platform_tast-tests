// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/uuid"

	// TODO: Change to "slices" when go is upgraded to 1.21 or later.
	"golang.org/x/exp/slices"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type jsonFwInfo struct {
	Board  string `json:"board_name"`
	Model  string `json:"model_name"`
	FwID   string `json:"firmware_build_cros_version"`
	Branch string `json:"branch_name"`
}

type apROBootabilityPerformanceArgs struct {
	targetProgrammer fwpb.Programmer
	imageSectionRW   fwpb.ImageSection
	imageSectionRO   fwpb.ImageSection
}

const (
	// firmwareFileName contains the name of the file when downloaded.
	firmwareFileName = "firmware_from_source.tar.bz2"

	// flashingTime sets the timeout for the flashing process.
	flashingTime = 20 * time.Minute

	// reconnectTime sets the timeout to reconnect DUT after flashing.
	reconnectTime = 10 * time.Minute

	// speedometerTime sets the timeout for Speedometer test.
	speedometerTime = 10 * time.Minute

	// deviationTarget contains the acceptable percentage of deviation from the baseline.
	deviationTarget = 0.10

	// maxSpeedTestRetry sets the maximum number of attempts to re-run the speed test in
	// case the result is found outside the expected deviation.
	maxSpeedTestRetry = 3

	// Name of the files that will contain the backup firmware.
	apFwBackup = "apFwBackup.bin"
	ecFwBackup = "ecFwBackup.bin"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: APROBootabilityPerformance,
		Desc: "Ensure bootability and system level performance with old RO AP builds",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_trial"},
		Vars:         []string{"firmware_branch", "ro_versions"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      90 * time.Minute, // 1hr30min.
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
		Data:         []string{"shipped-firmwares.json"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnModel("skolas")),
		Params: []testing.Param{{
			Val: &apROBootabilityPerformanceArgs{
				targetProgrammer: fwpb.Programmer_BIOSProgrammer,
				imageSectionRO:   fwpb.ImageSection_APROImageSection,
			},
		}},
	})
}

func APROBootabilityPerformance(ctx context.Context, s *testing.State) {
	/* The overall logic carried out by the test is:
	RO_old   + RW_old (EC_old) - this will be the baseline
	RO_old   + RW_new (EC_old) - compare it to baseline
	RO_old-1 + RW_new (EC_old-1) - compare it to baseline
	RO_old-2 + RW_new (EC_old-2) - compare it to baseline
	.
	.
	RO_old-n + RW_new (EC_old-n) - compare it to baseline
	RO_new   + RW_new (EC_new) - compare it to baseline
	*/
	testArgs := s.Param().(*apROBootabilityPerformanceArgs)

	// rwNewID and roNewID contain the AP RW and RO firmware version IDs available on the DUT.
	// These versions would be the to-be-qualified RW_new and RO_new firmware, respectively.
	var rwNewID, roNewID string

	// ecROID and ecRWID contain the EC RO and RW version IDs
	// available in the binary file to be flashed.
	var ecROID, ecRWID string

	// sectionNames is a map that converts ImageSection names
	// APRWA and APRWB to "A" and "B".
	sectionNames := map[fwpb.ImageSection]string{
		fwpb.ImageSection_APRWAImageSection: "A",
		fwpb.ImageSection_APRWBImageSection: "B",
	}

	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Failed to setup BiosServiceClient: ", err)
	}

	s.Log("Disabling hardware write protect")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to disable hardware write protect: ", err)
	}

	s.Log("Disabling EC software write protect")
	if err := h.Servo.RunECCommand(ctx, "flashwp disable now"); err != nil {
		s.Fatal("Failed to disable EC software write protect: ", err)
	}

	s.Log("Disabling AP software write protect")
	if _, err := h.BiosServiceClient.SetAPSoftwareWriteProtect(ctx, &fwpb.WPRequest{Enable: false}); err != nil {
		s.Fatal("Failed to disable AP software write protection: ", err)
	}

	// Give enough time for the deferred function to restore DUT.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 25*time.Minute)
	defer cancel()

	// Enable EC_SOFTWARE_SYNC at the end of the test if it was disabled.
	var ecSoftwareSyncDisabled bool
	var err error
	defer func() {
		if ecSoftwareSyncDisabled {
			s.Log("Enabling EC software sync")
			req := fwpb.GBBFlagsState{Clear: []fwpb.GBBFlag{fwpb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}
			if gbbFlagChanged, err := fwCommon.ClearAndSetGBBFlags(cleanupCtx, h.DUT, &req); err != nil {
				s.Fatal("Failed to enable EC software sync: ", err)
			} else if gbbFlagChanged {
				if err := safeReboot(cleanupCtx, h); err != nil {
					s.Fatal("While rebooting to restore gbb flags: ", err)
				}
			}
		}
	}()

	s.Log("Ensuring EC software sync is disabled")
	req := fwpb.GBBFlagsState{Set: []fwpb.GBBFlag{fwpb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}
	ecSoftwareSyncDisabled, err = fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req)
	if err != nil {
		s.Fatal("Failed to disable EC software sync: ", err)
	}
	if ecSoftwareSyncDisabled {
		s.Log("Disabling EC software sync")
		if err := safeReboot(ctx, h); err != nil {
			s.Fatal("While rebooting to set gbb flags: ", err)
		}
	}

	// Get the model name from 'crossystem fwid'.
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

	// Get the initial active section.
	initialActSection, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
	if err != nil {
		s.Fatal("Failed to get crossystem mainfw_act: ", err)
	}

	// Verify h.Model is defined.
	if h.Model == "" {
		testing.ContextLogf(ctx, "WARNING! No h.Model defined for this DUT, setting it as %s", fwidModel)
		h.Model = fwidModel
	}

	// The 'SHIPPED' firmware IDs can be generated and exported to a json file
	// by running the following bq command:
	/*
		bq query --use_legacy_sql=false --format json -n 3000 --project_id=jeremys-scratch-project 'SELECT DISTINCT branch_name, board_name, model_name, firmware_build_cros_version
		FROM `google.com:cros-goldeneye.prod.FirmwareQuals`
		WHERE ship_status <> "NOT_SHIPPED" AND firmware_type <> "TYPE_RW" AND firmware_build_cros_version <> "null"
		ORDER BY board_name, model_name, firmware_build_cros_version' | json_pp > ~/chromiumos/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/data/shipped-firmwares.json
	*/
	// The json file was manually deposited as internal data under 'firmware/data'.

	var shippedFwVersions []jsonFwInfo
	fwBranchVar, fwBranchVarOk := s.Var("firmware_branch")
	roVersions, roVersionsOk := s.Var("ro_versions")
	if roVersionsOk && fwBranchVarOk {
		for _, ro := range strings.Split(roVersions, ",") {
			shippedFwVersions = append(shippedFwVersions, jsonFwInfo{
				FwID:   ro,
				Branch: fwBranchVar,
			})
		}
		s.Log("SHIPPED firmwares manually set from command line")
	} else {
		// Read from the 'shipped-firmwares.json' file.
		jsonFilePath := s.DataPath("shipped-firmwares.json")
		shippedFwVersions, err = collectShippedFws(h, jsonFilePath)
		if err != nil {
			s.Fatal("While collecting the shipped fw versions: ", err)
		}
		s.Logf("SHIPPED firmwares found for model %s:", h.Model)
	}

	// Sort the shipped firmware versions so that they will be flashed accordingly
        // from the latest to the oldest.
	if err := sortVersions(shippedFwVersions); err != nil {
		s.Fatal("Failed to sort the shipped fw versions: ", err)
	}

	for i := range shippedFwVersions {
		s.Log(shippedFwVersions[i].FwID)
	}

	// Create a new directory to store the downloaded files.
	tmpDir, err := os.MkdirTemp("", "firmware-APROBootabilityPerformance")
	if err != nil {
		s.Fatal("Failed to create a new directory for the test: ", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a new directory in servo host to store the downloaded files.
	randomSuffix, err := uuid.NewRandom()
	if err != nil {
		s.Fatal("Failed to create a random suffix: ", err)
	}
	tmpDirServo := fmt.Sprintf("/var/tmp/APROBootabilityPerformance-%s", randomSuffix)
	s.Logf("Servo tmp dir: %s", tmpDirServo)
	if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", tmpDirServo); err != nil {
		s.Fatalf("Failed to create temp directory %s on servo host: %s", tmpDirServo, err)
	}
	// Delete the tmp directory on the servo host at the end.
	defer func() {
		s.Log("Deleting tmp directory on servo: ", tmpDirServo)
		if err := h.ServoProxy.RunCommand(cleanupCtx, false, "rm", "-rf", tmpDirServo); err != nil {
			s.Fatal("Failed to delete temp directory on servo host: ", err)
		}
	}()

	restoreECOverServo := true
	ecChip, err := h.Servo.GetString(ctx, servo.ECChip)
	if err != nil {
		s.Fatal("Failed to read DUT EC Chip: ", err)
	}
	if strings.HasPrefix(ecChip, "it8") {
		// TODO(b/307797049) Remove this condition once the issue with flash_ec is resolved.
		// In normal circumstances, this test performs the EC flash by sending the bin files
		// to the DUT and conducting a local flash. However, if afterward, the DUT is unable
		// to boot, as a recovery measure an external flash attempt will be made using the
		// servo to prevent the DUT from being left unbootable. Due to some observed issues
		// with using servo to flash it8 chips, skip restoring EC for now on those chips.
		s.Log("Skip restoring EC over servo on EC chip: ", ecChip)
		restoreECOverServo = false
	}

	// Download the latest shipped firmware.
	firmwareFilesToFlash, err := downloadAndUntarFwFile(ctx, s, h, tmpDir, tmpDirServo, fwidModel, shippedFwVersions[len(shippedFwVersions)-1])
	if err != nil {
		s.Fatal("Failed while downloading file: ", err)
	}

	// Back up current EC firmware. AP firmware is handled by fixture.
	s.Log("Backing up EC firmware")
	ecBackupData, err := h.BiosServiceClient.BackupImageSection(ctx, &fwpb.FWSectionInfo{Section: fwpb.ImageSection_EmptyImageSection, Programmer: fwpb.Programmer_ECProgrammer})
	if err != nil {
		s.Fatal("Failed to backup current EC firmware: ", err)
	}

	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed creating remote temp dir: ", err)
	}
	dutTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		h.DUT.Conn().CommandContext(cleanupCtx, "rm", "-r", dutTempDir)
	}()
	apBackupOnDut := fmt.Sprintf("%s/bios_backup.bin", dutTempDir)
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, apBackupOnDut); err != nil {
		s.Fatal("Failed to copy firmware image to DUT: ", err)
	}

	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		s.Fatal("Failed to setup futility instance: ", err)
	}

	apSections, out, err := futilityInstance.DumpFmap(ctx, apBackupOnDut, []string{string(bios.RWFWIDAImageSection), string(bios.RWFWIDBImageSection)})
	if err != nil {
		s.Fatal("Failed to get FMap sections: ", err, "\nOutput:\n", string(out))
	}

	rwA := apSections[slices.IndexFunc(apSections, func(s futility.FMapSection) bool { return s.Name == string(bios.RWFWIDAImageSection) })]
	rwB := apSections[slices.IndexFunc(apSections, func(s futility.FMapSection) bool { return s.Name == string(bios.RWFWIDBImageSection) })]

	ecSections, out, err := futilityInstance.DumpFmap(ctx, ecBackupData.Path, []string{string(bios.ROFRIDImageSection), string(bios.RWFWIDImageSection)})
	if err != nil {
		s.Fatal("Failed to get FMap sections: ", err, "\nOutput:\n", string(out))
	}
	ecRO := ecSections[slices.IndexFunc(ecSections, func(s futility.FMapSection) bool { return s.Name == string(bios.ROFRIDImageSection) })]
	ecRW := ecSections[slices.IndexFunc(ecSections, func(s futility.FMapSection) bool { return s.Name == string(bios.RWFWIDImageSection) })]

	s.Log("Saving the AP firmware")
	apBackupOnHost := filepath.Join(tmpDir, apFwBackup)
	if err := backupManager.CopyBackupFile(fixture.FirmwareAP, apBackupOnHost); err != nil {
		s.Fatal("Failed to save AP backup file on host: ", err)
	}
	s.Log("Saving the EC firmware")
	ecBackupOnHost := filepath.Join(tmpDir, ecFwBackup)
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), ecBackupData.Path, ecBackupOnHost, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to save EC backup file on host: ", err)
	}

	// Get the newest RW firmware version ID available on the DUT.
	// This version would be the to-be-qualified RW_new firmware.
	rwNewID, testArgs.imageSectionRW, err = getNewestRWIDAvailable(ctx, filepath.Join(tmpDir, apFwBackup), rwA, rwB)
	if err != nil {
		s.Fatal("Failed while dissecting the bin file: ", err)
	}
	s.Logf("Setting RW ID = %s, from section = %s as the to-be-qualified RW_new firmware", rwNewID, sectionNames[testArgs.imageSectionRW])

	// Get the RO firmware version ID available on the DUT.
	roNewID, err = firmware.GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		s.Fatal("Failed to get AP RO ID: ", err)
	}
	s.Logf("Setting RO ID = %s, as the to-be-qualified RO_new firmware", roNewID)

	// At the end of this test, restore firmware to the one found at the beginning.
	defer func(ctx context.Context, roNewID, initialRwFwid, initialActSection, ecChip string, testArgs *apROBootabilityPerformanceArgs) {
		if !h.DUT.Connected(ctx) && restoreECOverServo {
			// If a DUT reaches this point unable to boot, attempt to restore
			// the EC firmware through servo.
			// To-Do: Add restoration of AP when a DUT can't boot.
			s.Log("Restoring EC firmware through servo at the end of the test")
			fileMap := map[string]string{fmt.Sprintf("%s/%s", tmpDir, ecFwBackup): fmt.Sprintf("%s/%s", tmpDirServo, ecFwBackup)}
			if err := h.ServoProxy.PutFiles(ctx, false, fileMap); err != nil {
				s.Fatal("Failed to copy EC firmware file to servo host: ", err)
			}
			flashEcArgs := []string{fmt.Sprintf("--chip=%s", ecChip), fmt.Sprintf("--image=%s/%s", tmpDirServo, ecFwBackup), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort()), "--verify", "--verbose"}
			if ecChip == "stm32" {
				flashEcArgs = append(flashEcArgs, "--bitbang_rate=57600")
			}
			if err := h.ServoProxy.RunCommand(ctx, false, "flash_ec", flashEcArgs...); err != nil {
				s.Fatalf("Failed to restore %s EC firmware: %v", ecChip, err)
			}

			if err := h.EnsureDUTBooted(ctx); err != nil {
				s.Fatal("Failed to ensure DUT connected at the end of test before restoring firmware: ", err)
			}

			// Ensure there is a functional RPC connection.
			h.CloseRPCConnection(ctx)
			if err := h.RequireBiosServiceClient(ctx); err != nil {
				s.Fatal("Failed to open RPC client for restoration: ", err)
			}
		}

		s.Log("Restoring firmware at the end of the test")
		if _, _, err = flashDUTAndReboot(ctx, h, apBackupOnHost, ecBackupOnHost, "", dutTempDir, fwpb.ImageSection_EmptyImageSection, ecRO, ecRW); err != nil {
			s.Fatal("Failed while flashing DUT to restore firmware at the end of test: ", err)
		}

		bootingSection, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Fatal("Failed to get the active firmware section: ", err)
		}

		// Ensuring that DUT ends up running the initial RW active section.
		if bootingSection != initialActSection {
			if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "fw_try_next="+initialActSection).Run(ssh.DumpLogOnError); err != nil {
				s.Fatal("Failed to set crossystem fw_try_next: ", err)
			}

			if err := safeReboot(ctx, h); err != nil {
				s.Fatal("While rebooting at the end of the test: ", err)
			}
		}

		if err = firmware.VerifyFwIDs(ctx, h, roNewID, initialRwFwid); err != nil {
			s.Fatal("Failed while verifying firmware IDs after flashing at the end of test: ", err)
		}
	}(cleanupCtx, roNewID, initialRwFwid, initialActSection, ecChip, testArgs)

	// Flash the latest shipped RO and RW firmware.
	s.Log("Setting RO_old + RW_old (EC_old)")
	if ecROID, ecRWID, err = flashDUTAndReboot(ctx, h, filepath.Join(tmpDir, firmwareFilesToFlash.APFirmwareFile), filepath.Join(tmpDir, firmwareFilesToFlash.ECFirmwareFile), filepath.Join(tmpDir, firmwareFilesToFlash.MonitorFile), dutTempDir, fwpb.ImageSection_EmptyImageSection, ecRO, ecRW); err != nil {
		s.Fatalf("Failed to flash RO_old + RW_old ( %s + %s ) [EC_old](%s,%s): %v", shippedFwVersions[len(shippedFwVersions)-1].FwID, shippedFwVersions[len(shippedFwVersions)-1].FwID, ecROID, ecRWID, err)
	}

	// Verify RO/RW firmware versions are the latest shipped firmware after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err = firmware.VerifyFwIDs(ctx, h, shippedFwVersions[len(shippedFwVersions)-1].FwID, shippedFwVersions[len(shippedFwVersions)-1].FwID); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", shippedFwVersions[len(shippedFwVersions)-1].FwID, shippedFwVersions[len(shippedFwVersions)-1].FwID, err)
	}

	s.Log("Performing the speed test")
	baseline, err := speedTest(ctx, h)
	if err != nil {
		s.Fatal("Failed to perform Speedometer test: ", err)
	}
	s.Logf("Setting the baseline as: %f", baseline)

	// Skip speedometer test if the RW_new firmware is the same as the
	// RO_old shipped version because this was already verified and set as baseline.
	if shippedFwVersions[len(shippedFwVersions)-1].FwID == rwNewID {
		s.Log("WARNING! Speed test skipped because RW_new is the same as RO_old. Already verified")
	} else {
		// Setting DUT to boot from the RW section that contains the newest firmware ID.
		// This will assure that the DUT will try to boot from the flashed section.
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "fw_try_next="+sectionNames[testArgs.imageSectionRW]).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set crossystem fw_try_next: ", err)
		}

		// Flashing RW_new firmware obtained from the DUT at the beginning of the test into RW section A.
		// This will leave the DUT with the latest RO shipped fw and
		// the to-be-qualified new RW firmware (i.e., RO_old + RW_new).
		s.Log("Setting RO_old + RW_new (EC_old)")
		if _, _, err = flashDUTAndReboot(ctx, h, apBackupOnHost, "", "", dutTempDir, testArgs.imageSectionRW, ecRO, ecRW); err != nil {
			s.Fatalf("Failed to flash RO_old + RW_new ( %s + %s ) [EC_old](%s,%s): %v", shippedFwVersions[len(shippedFwVersions)-1].FwID, rwNewID, ecROID, ecRWID, err)
		}

		// Verify that the RO firmware has not been modified and RW has the RW_new after the flashing process.
		if err := firmware.VerifyFwIDs(ctx, h, shippedFwVersions[len(shippedFwVersions)-1].FwID, rwNewID); err != nil {
			s.Fatalf("After flashing RO_old + RW_new ( %s + %s ): %v", shippedFwVersions[len(shippedFwVersions)-1].FwID, rwNewID, err)
		}

		s.Log("Performing the speed test")
		speedResult, err := speedTest(ctx, h)
		if err != nil {
			s.Fatal("Failed to perform Speedometer test: ", err)
		}

		// Check that the result deviation from the baseline is acceptable.
		if err = checkDeviation(ctx, h, baseline, speedResult); err != nil {
			s.Fatalf("Deviation with RO_old + RO_new ( %s + %s ) failed: %v", shippedFwVersions[len(shippedFwVersions)-1], rwNewID, err)
		}
	}

	// Repeat steps for older RO firmware versions (i.e., RO_old-n + RW_new).
	for i := len(shippedFwVersions) - 2; i >= 0; i-- {
		s.Log("Downloading an older shipped firmware file")
		firmwareFilesToFlash, err = downloadAndUntarFwFile(ctx, s, h, tmpDir, tmpDirServo, fwidModel, shippedFwVersions[i])
		if err != nil {
			s.Fatal("Failed while downloading file: ", err)
		}

		n := len(shippedFwVersions) - i - 1
		s.Logf("Setting RO_old-%d + RW_new (EC_old-%d)", n, n)
		if ecROID, ecRWID, err = flashDUTAndReboot(ctx, h, filepath.Join(tmpDir, firmwareFilesToFlash.APFirmwareFile), filepath.Join(tmpDir, firmwareFilesToFlash.ECFirmwareFile), filepath.Join(tmpDir, firmwareFilesToFlash.MonitorFile), dutTempDir, testArgs.imageSectionRO, ecRO, ecRW); err != nil {
			s.Fatalf("Failed to flash RO_old-%d + RW_new ( %s + %s ) [EC_old-%d](%s,%s): %v", n, shippedFwVersions[i].FwID, rwNewID, n, ecROID, ecRWID, err)
		}

		s.Log("Verifying the firmware versions after flash")
		if err := firmware.VerifyFwIDs(ctx, h, shippedFwVersions[i].FwID, rwNewID); err != nil {
			s.Fatalf("After flashing RO_old-%d + RW_new ( %s + %s): %v", len(shippedFwVersions)-i-1, shippedFwVersions[i].FwID, rwNewID, err)
		}

		s.Log("Performing the speed test")
		speedResult, err := speedTest(ctx, h)
		if err != nil {
			s.Fatal("Failed to perform Speedometer test: ", err)
		}

		s.Log("Checking that the result deviation from the baseline is acceptable")
		if err = checkDeviation(ctx, h, baseline, speedResult); err != nil {
			s.Fatalf("Deviation with RO_old-%d + RO_new ( %s + %s ) failed: %v", len(shippedFwVersions)-i-1, shippedFwVersions[i], rwNewID, err)
		}
	}

	if shippedFwVersions[len(shippedFwVersions)-1].FwID == roNewID {
		s.Log("WARNING! Speed test skipped because RO_new is the same as RO_old. Already verified")
	} else {
		// Testing scenario RO/RW with the to-be-qualified firmware (i.e., RO_new + RW_new).
		s.Log("Setting RO_new + RW_new (EC_new)")
		if ecROID, ecRWID, err = flashDUTAndReboot(ctx, h, apBackupOnHost, ecBackupOnHost, "", dutTempDir, testArgs.imageSectionRO, ecRO, ecRW); err != nil {
			s.Fatalf("Failed to flash RO_new + RW_new ( %s + %s ) [EC_new](%s,%s): %v", roNewID, rwNewID, ecROID, ecRWID, err)
		}

		s.Log("Verifying the firmware versions are the to-be-qualified new RO/RW after flash")
		if err := firmware.VerifyFwIDs(ctx, h, roNewID, rwNewID); err != nil {
			s.Fatalf("After flashing RO_new + RW_new ( %s + %s): %v", roNewID, rwNewID, err)
		}

		s.Log("Performing the speed test")
		speedResult, err := speedTest(ctx, h)
		if err != nil {
			s.Fatal("Failed to perform Speedometer test: ", err)
		}

		s.Log("Checking that the result deviation from the baseline is acceptable")
		if err := checkDeviation(ctx, h, baseline, speedResult); err != nil {
			s.Fatalf("Deviation with RO_new + RO_new ( %s + %s ) failed: %v", roNewID, rwNewID, err)
		}
	}
}

// areVersionsDescending accepts two version numbers and checks whether they are in the descending order.
func areVersionsDescending(fwIDA, fwIDB string) (bool, error) {
	splitA := strings.Split(fwIDA, ".")
	splitB := strings.Split(fwIDB, ".")
	for i := 0; i < len(splitA); i++ {
		var idA, idB int
		if _, err := fmt.Sscanf(splitA[i], "%d", &idA); err != nil {
			return false, errors.Wrapf(err, "failed to sscanf %s", splitA[i])
		}
		if _, err := fmt.Sscanf(splitB[i], "%d", &idB); err != nil {
			return false, errors.Wrapf(err, "failed to sscanf %s", splitB[i])
		}
		if idB < idA {
			return true, nil
		}
	}
	return false, nil
}

// sortVersions accepts jsonFwInfo, and sorts the FwId version numbers.
func sortVersions(shippedFwInfos []jsonFwInfo) error {
	for i := range shippedFwInfos {
		for j := 0; j < len(shippedFwInfos)-i-1; j++ {
			areDesc, err := areVersionsDescending(shippedFwInfos[j].FwID, shippedFwInfos[j+1].FwID)
			if areDesc {
				shippedFwInfos[j], shippedFwInfos[j+1] = shippedFwInfos[j+1], shippedFwInfos[j]
			}
			if err != nil {
				return errors.Wrap(err, "failed to sort versions")
			}
		}
	}
	return nil
}

// downloadAndUntarFwFile downloads and untars a firmware source file from the cloud,
// using the given model name and shipped firmware version. It returns the path to the
// untarred firmware binary on the host.
func downloadAndUntarFwFile(ctx context.Context, s *testing.State, h *firmware.Helper, tmpDir, tmpDirServo, fwidModel string, fwToTest jsonFwInfo) (*firmware.FWFilesToFlash, error) {
	// getValidURL runs 'gsutil ls' and returns the valid url containing the firmware source.
	getValidURL := func(path string) (string, string, error) {
		out, stderr, err := testexec.CommandContext(ctx, "gsutil", "ls", path).SeparatedOutput(testexec.DumpLogOnError)
		if err != nil {
			if !strings.Contains(string(stderr), "One or more URLs matched no objects.") {
				return "", "", errors.Wrapf(err, "failed to run 'gsutil ls %s' to find the complete path: %s", path, stderr)
			}
			testing.ContextLogf(ctx, "Path not found for board %q", fwToTest.Board)
			return "", "", nil
		}

		// Regular expression to match for the required firmware id.
		re := regexp.MustCompile(`[R].*-` + fwToTest.FwID)
		releasedFWid := re.FindString(string(out))
		if releasedFWid == "" {
			testing.ContextLogf(ctx, "No matches found for firmware id: %s board: %s", fwToTest.FwID, fwToTest.Board)
			return "", "", nil
		}

		// There may be different URL patterns, under which the firmware file sits,
		// from model to model, and from version to version for the same model.
		var completeURL, completeFileName string
		if path == "gs://chromeos-releases/canary-channel/"+fwToTest.Board+"/"+fwToTest.FwID {
			completeURL = path
			completeFileName = "ChromeOS-firmware-" + releasedFWid + "-" + fwToTest.Board + ".tar.bz2"
		} else {
			// Identify if there is a sub-directory with the name of the board.
			out, stderr, err = testexec.CommandContext(ctx, "gsutil", "ls", path+"/"+releasedFWid).SeparatedOutput(testexec.DumpLogOnError)
			if err != nil {
				return "", "", errors.Wrapf(err, "failed to run 'gsutil ls' to check for sub-directories: %s:", stderr)
			}
			re = regexp.MustCompile(`.*` + releasedFWid + `/` + fwToTest.Board)
			completeURL = re.FindString(string(out))
			if completeURL == "" {
				completeURL = path + "/" + releasedFWid
				completeFileName = firmwareFileName
			} else {
				completeFileName = firmwareFileName
			}
		}
		return completeURL, completeFileName, nil
	}

	// List of possible paths that contain the firmware_from_source.tar.bz2 file.
	pathsPool := []string{
		/*
			Optional path to get firmware files:
			gs://chromeos-image-archive/zork-firmware/R87-13434.635.0/
		*/
		"gs://chromeos-image-archive/" + fwToTest.Board + "-firmware",

		/*
			Use the branch name obtained from the json file to download the firmware file:
			gs://chromeos-image-archive/firmware-kukui-12573.B-branch-firmware/R79-12573.342.0/
		*/
		"gs://chromeos-image-archive/" + fwToTest.Branch + "-branch-firmware",

		/*
			Drawper, drawcia, drawlat and drawman firmware file for release R89-13606.485.0 can be downloaded from:
			gs://chromeos-releases/canary-channel/dedede/13606.485.0/
		*/
		"gs://chromeos-releases/canary-channel/" + fwToTest.Board + "/" + fwToTest.FwID,
	}

	for _, path := range pathsPool {
		testing.ContextLogf(ctx, "Using path: %s", path)
		url, fileName, err := getValidURL(path)
		if err != nil {
			return nil, err
		}

		if url != "" {
			filesToFlash, err := firmware.DownloadFirmwareFiles(ctx, s.CloudStorage(), h, tmpDirServo, url, fileName, fwidModel)
			if err != nil {
				testing.ContextLog(ctx, "Failed to download the file: ", err)
				continue
			}
			// Copy firmware files to the local host.
			if filesToFlash.APFirmwareFile != "" {
				if err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", tmpDirServo, filesToFlash.APFirmwareFile), fmt.Sprintf("%s/%s", tmpDir, filesToFlash.APFirmwareFile)); err != nil {
					return nil, errors.Wrap(err, "failed to copy AP firmware file from servo host")
				}
			}
			if filesToFlash.ECFirmwareFile != "" {
				if err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", tmpDirServo, filesToFlash.ECFirmwareFile), fmt.Sprintf("%s/%s", tmpDir, filesToFlash.ECFirmwareFile)); err != nil {
					return nil, errors.Wrap(err, "failed to copy EC firmware file from servo host")
				}
			}
			if filesToFlash.MonitorFile != "" {
				if err := h.ServoProxy.GetFile(ctx, false, fmt.Sprintf("%s/%s", tmpDirServo, filesToFlash.MonitorFile), fmt.Sprintf("%s/%s", tmpDir, filesToFlash.MonitorFile)); err != nil {
					return nil, errors.Wrap(err, "failed to copy Monitor firmware file from servo host")
				}
			}
			return filesToFlash, nil
		}
	}

	return nil, errors.Errorf("unable to get firmware file for board: %s, model: %s, firmware ID: %s", fwToTest.Board, fwToTest.Model, fwToTest.FwID)
}

// flashDUTAndReboot will send the bin files to a directory in the DUT, flash the files into the DUT with the bios service 'WriteImageFromMultiSectionFile'
// and reboot the DUT so that the flash takes effect.
func flashDUTAndReboot(ctx context.Context, h *firmware.Helper, apFile, ecFile, monitorFile, dutTempDir string, apSection fwpb.ImageSection, ecRO, ecRW futility.FMapSection) (ecROID, ecRWID string, err error) {
	flashingCtx, cancelflashingCtx := context.WithTimeout(ctx, flashingTime)
	defer cancelflashingCtx()

	if ecFile != "" {
		filePathOnDut := filepath.Join(dutTempDir, filepath.Base(ecFile))
		testing.ContextLogf(flashingCtx, "Sending EC image %q to DUT to %q", ecFile, filePathOnDut)
		if _, err := linuxssh.PutFiles(flashingCtx, h.DUT.Conn(), map[string]string{ecFile: filePathOnDut}, linuxssh.DereferenceSymlinks); err != nil {
			return "", "", errors.Wrap(err, "failed to send bin file to DUT")
		}
		if monitorFile != "" {
			monitorOnDut := filepath.Join(dutTempDir, filepath.Base(monitorFile))
			testing.ContextLogf(flashingCtx, "Sending EC Monitor image %q to DUT to %q", monitorFile, monitorOnDut)
			if _, err := linuxssh.PutFiles(flashingCtx, h.DUT.Conn(), map[string]string{monitorFile: monitorOnDut}, linuxssh.DereferenceSymlinks); err != nil {
				return "", "", errors.Wrap(err, "failed to send bin file to DUT")
			}
		}
		// Getting the EC RO and RW versions from the binary file.
		ecROID, ecRWID, err = getFWIDFromBinFile(ctx, ecFile, ecRO, ecRW)
		if err != nil {
			return "", "", errors.Wrap(err, "failed to get EC IDs")
		}
		testing.ContextLogf(flashingCtx, "Flashing DUT with file: %s using section: %v", ecFile, apSection)
		if _, err := h.BiosServiceClient.WriteImageFromMultiSectionFile(flashingCtx, &fwpb.FWSectionInfo{Programmer: fwpb.Programmer_ECProgrammer, Path: filePathOnDut, Section: fwpb.ImageSection_EmptyImageSection}); err != nil {
			return ecROID, ecRWID, errors.Wrap(err, "failed to flash DUT with the multi-section bin file")
		}
	}

	if apFile != "" {
		filePathOnDut := filepath.Join(dutTempDir, filepath.Base(apFile))
		testing.ContextLogf(flashingCtx, "Sending AP image %q to DUT to %q", apFile, filePathOnDut)
		if _, err := linuxssh.PutFiles(flashingCtx, h.DUT.Conn(), map[string]string{apFile: filePathOnDut}, linuxssh.DereferenceSymlinks); err != nil {
			return "", "", errors.Wrap(err, "failed to send bin file to DUT")
		}
		testing.ContextLogf(flashingCtx, "Flashing DUT with file: %s using section: %v", apFile, apSection)
		if _, err := h.BiosServiceClient.WriteImageFromMultiSectionFile(flashingCtx, &fwpb.FWSectionInfo{Programmer: fwpb.Programmer_BIOSProgrammer, Path: filePathOnDut, Section: apSection}); err != nil {
			return ecROID, ecRWID, errors.Wrap(err, "failed to flash DUT with the multi-section bin file")
		}
	}

	// Reboot DUT for flash to take effect.
	if err := safeReboot(flashingCtx, h); err != nil {
		return ecROID, ecRWID, errors.Wrap(err, "while rebooting after flash")
	}

	return ecROID, ecROID, nil
}

// safeReboot will close RPC connection, reboot DUT and Open a new RPC connection.
func safeReboot(ctx context.Context, h *firmware.Helper) error {
	// Close RPC connection before reboot.
	h.CloseRPCConnection(ctx)

	testing.ContextLog(ctx, "Power-cycling DUT with a cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to reboot DUT by servo")
	}

	testing.ContextLog(ctx, "Waiting for DUT to reconnect")
	connectCtx, cancelconnectCtx := context.WithTimeout(ctx, reconnectTime)
	defer cancelconnectCtx()

	if err := h.WaitConnect(connectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}

	// Setup BiosServiceClient again.
	if err := h.RequireBiosServiceClient(ctx); err != nil {
		return errors.Wrap(err, "failed to setup BiosServiceClient after reboot")
	}

	return nil
}

// speedTest performs the speedometer2 test.
func speedTest(ctx context.Context, h *firmware.Helper) (float64, error) {
	speedometerCtx, cancelSpeedometerCtx := context.WithTimeout(ctx, speedometerTime)
	defer cancelSpeedometerCtx()

	testing.ContextLog(speedometerCtx, "Sleeping for a few seconds before starting a new Chrome")
	// GoBigSleepLint: Delay for the DUT to fully settle before starting a new chrome session.
	if err := testing.Sleep(speedometerCtx, 5*time.Second); err != nil {
		return 0.0, errors.Wrap(err, "failed to wait for a few seconds")
	}

	speedometerService := fwpb.NewUtilsServiceClient(h.RPCClient.Conn)
	if _, err := speedometerService.NewChrome(speedometerCtx, &empty.Empty{}); err != nil {
		return 0.0, errors.Wrap(err, "failed to initiate a chrome sesion")
	}
	defer func() error {
		if _, err := speedometerService.CloseChrome(speedometerCtx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to close the chrome sesion")
		}
		return nil
	}()

	testing.ContextLog(speedometerCtx, "Running speedometer test")
	sptest, err := speedometerService.PerformSpeedometerTest(speedometerCtx, &empty.Empty{})
	if err != nil {
		return 0.0, errors.Wrap(err, "failed while performing the Speedometer benchmark")
	}

	// Pars the output of the test as a float for later math operations.
	result, err := strconv.ParseFloat(sptest.Result, 64)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to convert the result into float")
	}

	testing.ContextLogf(speedometerCtx, "Speedometer Result: %f", result)
	return result, nil
}

// checkDeviation will verify if the result is inside the accepted deviation range.
func checkDeviation(ctx context.Context, h *firmware.Helper, baseline, result float64) error {
	deviation := (baseline * deviationTarget)
	upperBound := baseline + deviation
	lowerBound := baseline - deviation
	if result > upperBound {
		testing.ContextLogf(ctx, "Speedometer result %v is HIGHER than targeted deviation of %v from baseline %v", result, deviationTarget, baseline)
		return nil
	}
	var retrySpeedTest bool
	if result < lowerBound {
		testing.ContextLogf(ctx, "Speedometer result %v is LOWER than targeted deviation of %v from baseline %v", result, deviationTarget, baseline)
		retrySpeedTest = true
	}
	calculateAverage := func(nums []float64, n int) float64 {
		var sum float64 = 0
		for i := 0; i < n; i++ {
			sum += (nums[i])
		}
		avg := float64(sum) / float64(n)
		return avg
	}
	if retrySpeedTest {
		testing.ContextLog(ctx, "Retrying speed test")
		speedTestNums := []float64{result}
		var averageVal float64
		if err := func() error {
			for attempt := 1; attempt <= maxSpeedTestRetry; attempt++ {
				newVal, err := speedTest(ctx, h)
				if err != nil {
					return errors.Wrapf(err, "failed to perform speedometer test during retry %d", attempt)
				}
				speedTestNums = append(speedTestNums, newVal)
				averageVal = calculateAverage(speedTestNums, len(speedTestNums))
				if averageVal > lowerBound {
					return nil
				}
			}
			return errors.Errorf("got speed test average %v LOWER than targeted deviation of %v from baseline %v after %v attempts", averageVal, deviationTarget, baseline, maxSpeedTestRetry)
		}(); err != nil {
			return err
		}
	}
	testing.ContextLog(ctx, "Result is inside the limits of deviation")
	return nil
}

// collectShippedFws will parse the firmware IDs from the json file.
func collectShippedFws(h *firmware.Helper, filepath string) ([]jsonFwInfo, error) {
	out, err := os.ReadFile(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read JSON file")
	}

	var data []jsonFwInfo
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, errors.Wrap(err, "failed to parse JSON file")
	}

	var shippedFws []jsonFwInfo
	for _, values := range data {
		if values.Model == h.Model || values.Board == h.Model && values.Model == "" {
			shippedFws = append(shippedFws, values)
		}
	}

	if len(shippedFws) == 0 {
		return nil, errors.Errorf("did not find any shipped fw for %s", h.Model)
	}

	return shippedFws, nil
}

// getFWIDFromBinFile returns a string representation of the data pointed by sections from a bin file.
func getFWIDFromBinFile(ctx context.Context, pathToFile string, section1, section2 futility.FMapSection) (string, string, error) {
	// Open and read the bin file.
	binFile, err := os.Open(pathToFile)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to open file")
	}
	defer binFile.Close()

	fileInfo, err := binFile.Stat()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read file")
	}
	reader := bufio.NewReader(binFile)
	buf := make([]byte, fileInfo.Size())
	_, err = io.ReadFull(reader, buf)
	if err != nil {
		return "", "", errors.Wrap(err, "unexpected file read error")
	}

	// Get only the firmware IDs numbers from required sections.
	section1ID := strings.Trim(string(buf[section1.Offset:section1.Offset+section1.Size]), "\x00")
	section2ID := strings.Trim(string(buf[section2.Offset:section2.Offset+section2.Size]), "\x00")

	testing.ContextLogf(ctx, "Found ID = %s, in section = %s", section1ID, section1.Name)
	testing.ContextLogf(ctx, "Found ID = %s, in section = %s", section2ID, section2.Name)
	return section1ID, section2ID, nil
}

// getNewestRWIDAvailable identifies which is the newest firmware ID available
// in the DUT by dissecting the AP bin file.
func getNewestRWIDAvailable(ctx context.Context, pathToFile string, rwA, rwB futility.FMapSection) (string, fwpb.ImageSection, error) {
	apRWA, apRWB, err := getFWIDFromBinFile(ctx, pathToFile, rwA, rwB)
	if err != nil {
		return "", fwpb.ImageSection_EmptyImageSection, errors.Wrap(err, "failed to get ID from bin file")
	}
	splitRWA := strings.Split(apRWA, ".")
	splitRWB := strings.Split(apRWB, ".")

	// Compare the firmware IDs from section A and B to identify which is the newer.
	// If they are the same, use section A as default.
	rwDefaultStr := splitRWA[1] + "." + splitRWA[2] + "." + splitRWA[3]
	for i := 1; i < len(splitRWA); i++ {
		var idA, idB int
		if _, err := fmt.Sscanf(splitRWA[i], "%d", &idA); err != nil {
			return "", fwpb.ImageSection_EmptyImageSection, errors.Wrapf(err, "failed to sscanf %s", splitRWA[i])
		}
		if _, err := fmt.Sscanf(splitRWB[i], "%d", &idB); err != nil {
			return "", fwpb.ImageSection_EmptyImageSection, errors.Wrapf(err, "failed to sscanf %s", splitRWB[i])
		}

		if idB > idA {
			rwBStr := splitRWB[1] + "." + splitRWB[2] + "." + splitRWB[3]
			return rwBStr, fwpb.ImageSection_APRWBImageSection, nil
		}
		if idB < idA {
			return rwDefaultStr, fwpb.ImageSection_APRWAImageSection, nil
		}
	}
	return rwDefaultStr, fwpb.ImageSection_APRWAImageSection, nil
}
