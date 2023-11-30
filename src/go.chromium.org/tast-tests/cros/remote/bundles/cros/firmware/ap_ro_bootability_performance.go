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

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
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

// secInfo will contain the fmap information for a fw image section.
type secInfo struct {
	name   bios.ImageSection
	offset int64
	size   int64
}

type apROBootabilityPerformanceArgs struct {
	targetProgrammer fwpb.Programmer
	imageSectionRW   fwpb.ImageSection
	imageSectionRO   fwpb.ImageSection
}

const (
	// firmwareFileName contains the name of the file when downloaded.
	firmwareFileName = "firmware_from_source.tar.bz2"

	// fileOnDUTToFlash contains the path on the DUT, under which the firmware file to be tested
	// is copied from the host machine.
	fileOnDUTToFlash = "/tmp/firmwareForTest.bin"
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
)

var (
	// rwNewID contains the RW firmware version ID available on the DUT.
	// This version would be the to-be-qualified RW_new firmware.
	rwNewID string

	// roNewID contains the RO firmware version ID available on the DUT.
	// This version would be the to-be-qualified RO_new firmware.
	roNewID string
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
		Vars:         []string{"board", "model"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      90 * time.Minute, // 1hr30min.
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
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
	RO_old   + RW_old - this will be the baseline
	RO_old   + RW_new - compare it to baseline
	RO_old-1 + RW_new - compare it to baseline
	RO_old-2 + RW_new - compare it to baseline
	.
	.
	RO_old-n + RW_new - compare it to baseline
	RO_new   + RW_new - compare it to baseline
	*/
	testArgs := s.Param().(*apROBootabilityPerformanceArgs)

	// sectionNames is a map that converts ImageSection names
	// APRWA and APRWB to "A" and "B".
	sectionNames := map[fwpb.ImageSection]string{
		fwpb.ImageSection_APRWAImageSection: "A",
		fwpb.ImageSection_APRWBImageSection: "B",
	}

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// Confirm the CCD is open.
	if hasCCD, err := h.Servo.HasCCD(ctx); err != nil {
		s.Fatal("Failed while checking if servo has a CCD connection: ", err)
	} else if hasCCD {
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

	s.Log("Disabling EC software write protect")
	if err := h.Servo.RunECCommand(ctx, "flashwp disable now"); err != nil {
		s.Fatal("Failed to disable EC software write protect: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to open RPC client: ", err)
	}

	s.Log("Disabling AP software write protect")
	bs := fwpb.NewBiosServiceClient(h.RPCClient.Conn)
	if _, err := bs.SetAPSoftwareWriteProtect(ctx, &fwpb.WPRequest{Enable: false}); err != nil {
		s.Fatal("Failed to disable AP software write protection: ", err)
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
		bq query --use_legacy_sql=false --format json -n 3000 'SELECT DISTINCT branch_name, board_name, model_name, firmware_build_cros_version
		FROM `google.com:cros-goldeneye.prod.FirmwareQuals`
		WHERE ship_status <> "NOT_SHIPPED" AND firmware_type <> "TYPE_RW" AND firmware_build_cros_version <> "null"
		ORDER BY board_name, model_name, firmware_build_cros_version' | json_pp > ~/chromiumos/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/data/shipped-firmwares.json
	*/
	// The json file was manually deposited as internal data under 'firmware/data'.

	// Read from the 'shipped-firmwares.json' file.
	jsonFilePath := s.DataPath("shipped-firmwares.json")
	shippedFwVersions, err := collectShippedFws(h, jsonFilePath)
	if err != nil {
		s.Fatal("While collecting the shipped fw versions: ", err)
	}
	s.Logf("SHIPPED firmwares found for model %s:", h.Model)
	for i := range shippedFwVersions {
		s.Log(shippedFwVersions[i].FwID)
	}

	// Create a new directory to store the downloaded files.
	tmpDir, err := os.MkdirTemp("", "firmware-APROBootabilityPerformance")
	if err != nil {
		s.Fatal("Failed to create a new directory for the test: ", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download the latest shipped firmware.
	binToFlash, err := downloadAndUntarFwFile(ctx, s, tmpDir, fwidModel, shippedFwVersions[len(shippedFwVersions)-1])
	if err != nil {
		s.Fatal("Failed while downloading file: ", err)
	}

	// Back up a copy of the current AP firmware running on the DUT.
	s.Log("Backing up AP firmware")
	backupData, err := bs.BackupImageSection(ctx, &fwpb.FWSectionInfo{Section: fwpb.ImageSection_EmptyImageSection, Programmer: testArgs.targetProgrammer})
	if err != nil {
		s.Fatal("Failed to backup current AP firmware: ", err)
	}

	// Get the area_offset and area_size of the RWA and RWB section on the bin file.
	rwA, err := getOffsetSizeName(ctx, s.DUT().Conn(), backupData.Path, bios.RWFWIDAImageSection)
	if err != nil {
		s.Fatal("Failed to get fmap info for section A: ", err)
	}
	rwB, err := getOffsetSizeName(ctx, s.DUT().Conn(), backupData.Path, bios.RWFWIDBImageSection)
	if err != nil {
		s.Fatal("Failed to get fmap info for section B: ", err)
	}

	// Create initialFwFromDUT on the host machine, and copy the
	// DUT's currently running firmware to this file. This firmware
	// is also referred to as the to-be-qualified RO_new/RW_new firmware.
	initialFwFromDUT, err := os.CreateTemp(tmpDir, "")
	if err != nil {
		s.Fatal("Failed to create a file to store the backup on host: ", err)
	}
	defer initialFwFromDUT.Close()

	// Store the backup file in a temporary directory with the name 'initialFWOnDUT'.
	s.Log("Saving the AP firmware")
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), backupData.Path, initialFwFromDUT.Name(), linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to save the backup file on host: ", err)
	}

	// Get the newest RW firmware version ID available on the DUT.
	// This version would be the to-be-qualified RW_new firmware.
	rwNewID, testArgs.imageSectionRW, err = getNewestRWIDAvailable(ctx, initialFwFromDUT, rwA, rwB)
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

	// Give enough time for the deferred function to restore DUT.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 21*time.Minute)
	defer cancel()

	// At the end of this test, restore AP firmware to the one found at the beginning.
	defer func(ctx context.Context, roNewID, initialRwFwid, initialActSection string, initialFwFromDUT *os.File, testArgs *apROBootabilityPerformanceArgs) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure DUT connected at the end of test before restoring firmware: ", err)
		}

		// Ensure there is a functional RPC connection.
		h.CloseRPCConnection(ctx)
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to open RPC client for restoration: ", err)
		}

		s.Log("Restoring firmware at the end of the test")
		if err := flashDUTAndReboot(ctx, h, s.DUT().Conn(), initialFwFromDUT.Name(), fwpb.ImageSection_EmptyImageSection, testArgs.targetProgrammer); err != nil {
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
	}(cleanupCtx, roNewID, initialRwFwid, initialActSection, initialFwFromDUT, testArgs)

	// Flash the latest shipped RO and RW firmware.
	if err := flashDUTAndReboot(ctx, h, s.DUT().Conn(), binToFlash, fwpb.ImageSection_EmptyImageSection, testArgs.targetProgrammer); err != nil {
		s.Fatalf("Failed to flash RO_old + RW_old ( %s + %s ): %v", shippedFwVersions[len(shippedFwVersions)-1], shippedFwVersions[len(shippedFwVersions)-1], err)
	}

	// Verify RO/RW firmware versions are the latest shipped firmware after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err = firmware.VerifyFwIDs(ctx, h, shippedFwVersions[len(shippedFwVersions)-1].FwID, shippedFwVersions[len(shippedFwVersions)-1].FwID); err != nil {
		s.Fatalf("After flashing RO_old + RW_old ( %s + %s ): %v", shippedFwVersions[len(shippedFwVersions)-1], shippedFwVersions[len(shippedFwVersions)-1], err)
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
		s.Log("Flashing the to-be-qualified new RW firmware")
		if err := flashDUTAndReboot(ctx, h, s.DUT().Conn(), initialFwFromDUT.Name(), testArgs.imageSectionRW, testArgs.targetProgrammer); err != nil {
			s.Fatalf("Failed to flash RO_old + RW_new ( %s + %s ): %v", shippedFwVersions[len(shippedFwVersions)-1], rwNewID, err)
		}

		// Verify that the RO firmware has not been modified and RW has the RW_new after the flashing process.
		if err := firmware.VerifyFwIDs(ctx, h, shippedFwVersions[len(shippedFwVersions)-1].FwID, rwNewID); err != nil {
			s.Fatalf("After flashing RO_old + RW_new ( %s + %s ): %v", shippedFwVersions[len(shippedFwVersions)-1], rwNewID, err)
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
		binToFlash, err := downloadAndUntarFwFile(ctx, s, tmpDir, fwidModel, shippedFwVersions[i])
		if err != nil {
			s.Fatal("Failed while downloading file: ", err)
		}

		s.Log("Flashing the older RO 'shipped' firmware")
		if err := flashDUTAndReboot(ctx, h, s.DUT().Conn(), binToFlash, testArgs.imageSectionRO, testArgs.targetProgrammer); err != nil {
			s.Fatalf("Failed to flash RO_old-%d + RW_new ( %s + %s ): %v", len(shippedFwVersions)-i-1, shippedFwVersions[i], rwNewID, err)
		}

		s.Log("Verifying the firmware versions after flash")
		if err := firmware.VerifyFwIDs(ctx, h, shippedFwVersions[i].FwID, rwNewID); err != nil {
			s.Fatalf("After flashing RO_old-%d + RW_new ( %s + %s): %v", len(shippedFwVersions)-i-1, shippedFwVersions[i], rwNewID, err)
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
		s.Log("Flashing the to-be-qualified new RO firmware")
		if err = flashDUTAndReboot(ctx, h, s.DUT().Conn(), initialFwFromDUT.Name(), testArgs.imageSectionRO, testArgs.targetProgrammer); err != nil {
			s.Fatalf("Failed to flash RO_new + RW_new ( %s + %s ): %v", roNewID, rwNewID, err)
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

// downloadAndUntarFwFile downloads and untars a firmware source file from the cloud,
// using the given model name and shipped firmware version. It returns the path to the
// untarred firmware binary on the host.
func downloadAndUntarFwFile(ctx context.Context, s *testing.State, tmpDir, fwidModel string, fwToTest jsonFwInfo) (string, error) {
	// getValidURL runs 'gsutil ls' and returns the valid url containing the firmware source.
	getValidURL := func(path string) (string, error) {
		out, stderr, err := testexec.CommandContext(ctx, "gsutil", "ls", path).SeparatedOutput(testexec.DumpLogOnError)
		if err != nil {
			if !strings.Contains(string(stderr), "One or more URLs matched no objects.") {
				return "", errors.Wrapf(err, "failed to run 'gsutil ls %s' to find the complete path: %s", path, stderr)
			}
			testing.ContextLogf(ctx, "Path not found for board %q", fwToTest.Board)
			return "", nil
		}

		// Regular expression to match for the required firmware id.
		re := regexp.MustCompile(`[R].*-` + fwToTest.FwID)
		releasedFWid := re.FindString(string(out))
		if releasedFWid == "" {
			testing.ContextLogf(ctx, "No matches found for firmware id: %s board: %s", fwToTest.FwID, fwToTest.Board)
			return "", nil
		}

		// There may be different URL patterns, under which the firmware file sits,
		// from model to model, and from version to version for the same model.
		var completeURL string
		if path == "gs://chromeos-releases/canary-channel/"+fwToTest.Board+"/"+fwToTest.FwID+"/" {
			partialFirmwareFileName := "ChromeOS-firmware-" + releasedFWid + "-" + fwToTest.Board
			completeURL = path + partialFirmwareFileName + ".tar.bz2"
		} else {
			// Identify if there is a sub-directory with the name of the board.
			out, stderr, err = testexec.CommandContext(ctx, "gsutil", "ls", path+"/"+releasedFWid).SeparatedOutput(testexec.DumpLogOnError)
			if err != nil {
				return "", errors.Wrapf(err, "failed to run 'gsutil ls' to check for sub-directories: %s:", stderr)
			}
			re = regexp.MustCompile(`.*` + releasedFWid + `/` + fwToTest.Board)
			completeURL = re.FindString(string(out))
			if completeURL == "" {
				completeURL = path + "/" + releasedFWid + "/" + firmwareFileName
			} else {
				completeURL = completeURL + "/" + firmwareFileName
			}
		}
		return completeURL, nil
	}

	// downloadFwFromURL stages and downloads the firmware file from the given URL.
	downloadFwFromURL := func(url string) bool {
		// Use the url without the prefix 'gs://'.
		if err := firmware.DownloadFirmwareFile(ctx, s.CloudStorage(), tmpDir, url[5:]); err != nil {
			testing.ContextLog(ctx, "Failed to download the file: ", err)
			return false
		}
		return true
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
		"gs://chromeos-releases/canary-channel/" + fwToTest.Board + "/" + fwToTest.FwID + "/",
	}

	for _, path := range pathsPool {
		testing.ContextLogf(ctx, "Using path: %s", path)
		url, err := getValidURL(path)
		if err != nil {
			return "", err
		}

		if url != "" {
			if downloadFwFromURL(url) {
				binToFlash, _, err := firmware.UntarUnknownFileName(ctx, tmpDir, fwidModel, firmware.APFirmware)
				if err != nil {
					testing.ContextLogf(ctx, "Unable to untar the firmware file for board: %s, model: %s, firmware ID: %s", fwToTest.Board, fwToTest.Model, fwToTest.FwID)
				} else {
					return filepath.Join(tmpDir, binToFlash), nil
				}
			}
		}
	}

	return "", errors.Errorf("unable to get firmware file for board: %s, model: %s, firmware ID: %s", fwToTest.Board, fwToTest.Model, fwToTest.FwID)
}

// flashDUTAndReboot will send the bin files to a directory in the DUT, flash the files into the DUT with the bios service 'WriteImageFromMultiSectionFile'
// and reboot the DUT so that the flash takes effect.
func flashDUTAndReboot(ctx context.Context, h *firmware.Helper, conn *ssh.Conn, fileOnHostToFlash string, section fwpb.ImageSection, targetProgrammer fwpb.Programmer) error {
	flashingCtx, cancelflashingCtx := context.WithTimeout(ctx, flashingTime)
	defer cancelflashingCtx()

	testing.ContextLog(flashingCtx, "Sending firmware bin file to DUT")
	if _, err := linuxssh.PutFiles(flashingCtx, conn, map[string]string{fileOnHostToFlash: fileOnDUTToFlash}, linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrap(err, "failed to send bin file to DUT")
	}

	testing.ContextLogf(flashingCtx, "Flashing DUT with file: %s using section: %v", fileOnHostToFlash, section)
	bs := fwpb.NewBiosServiceClient(h.RPCClient.Conn)
	if _, err := bs.WriteImageFromMultiSectionFile(flashingCtx, &fwpb.FWSectionInfo{Programmer: targetProgrammer, Path: fileOnDUTToFlash, Section: section}); err != nil {
		return errors.Wrap(err, "failed to flash DUT with the multi-section bin file")
	}

	// Reboot DUT for flash to take effect.
	if err := safeReboot(flashingCtx, h); err != nil {
		return errors.Wrap(err, "while rebooting after flash")
	}

	return nil
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

	// Open RPC connection after reboot.
	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "failed to open RPC client after reboot")
	}

	return nil
}

// speedTest performs the speedometer2 test.
func speedTest(ctx context.Context, h *firmware.Helper) (float64, error) {
	speedometerCtx, cancelspeedometerCtx := context.WithTimeout(ctx, speedometerTime)
	defer cancelspeedometerCtx()

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
		if values.Model == h.Model {
			shippedFws = append(shippedFws, values)
		} else if values.Board == h.Model && values.Model == "" {
			shippedFws = append(shippedFws, values)
		}
	}

	if len(shippedFws) == 0 {
		return nil, errors.Errorf("did not find any shipped fw for %s", h.Model)
	}

	return shippedFws, nil
}

// getNewestRWIDAvailable identifies which is the newest firmware ID available
// in the DUT by dissecting the AP bin file.
func getNewestRWIDAvailable(ctx context.Context, initialFwFromDUT *os.File, rwA, rwB secInfo) (string, fwpb.ImageSection, error) {
	// Open and read the bin file.
	binFile, err := os.Open(initialFwFromDUT.Name())
	if err != nil {
		return "", fwpb.ImageSection_EmptyImageSection, errors.Wrap(err, "failed to open AP bin file")
	}
	defer binFile.Close()

	fileInfo, err := binFile.Stat()
	if err != nil {
		return "", fwpb.ImageSection_EmptyImageSection, errors.Wrap(err, "failed to read AP bin file")
	}
	reader := bufio.NewReader(binFile)
	buf := make([]byte, fileInfo.Size())
	for {
		_, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				return "", fwpb.ImageSection_EmptyImageSection, errors.Wrap(err, "unexpected error")
			}
			break
		}
	}
	// Get only the firmware IDs numbers from section A and B.
	splitRWA := strings.Split(strings.Trim(string(buf[rwA.offset:rwA.offset+rwA.size]), "\x00"), ".")
	splitRWB := strings.Split(strings.Trim(string(buf[rwB.offset:rwB.offset+rwB.size]), "\x00"), ".")

	testing.ContextLogf(ctx, "Found RW ID = %s, in section = %s", splitRWA, rwA.name)
	testing.ContextLogf(ctx, "Found RW ID = %s, in section = %s", splitRWB, rwB.name)

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

// getOffsetSizeName uses the dump_fmap command to get the area_offset, area_size and area_name of a bin file.
func getOffsetSizeName(ctx context.Context, conn *ssh.Conn, path string, section bios.ImageSection) (secInfo, error) {
	var data secInfo

	// Run dump_fmap command.
	out, err := conn.CommandContext(ctx, "fmap_decode", path).Output(ssh.DumpLogOnError)
	if err != nil {
		return data, errors.Wrap(err, "failed to run dump_fmap command")
	}
	areaRange := regexp.MustCompile(`area_offset=\"(0[xX][0-9a-fA-F]+)\" area_size=\"(0[xX][0-9a-fA-F]+)\"\s*area_name=\"` + string(section) + `\"`)
	match := areaRange.FindStringSubmatch(string(out))
	if len(match) != 3 {
		return data, errors.Wrapf(err, "failed to match regex %q in output: %s", areaRange, out)
	}
	offset, err := strconv.ParseInt(match[1], 0, 64)
	if err != nil {
		return data, errors.Wrapf(err, "failed to parse offset for section %q, got match: %s", section, match[1])
	}
	size, err := strconv.ParseInt(match[2], 0, 64)
	if err != nil {
		return data, errors.Wrapf(err, "failed to parse size for section %q, got match: %s", section, match[2])
	}
	data.name = section
	data.offset = offset
	data.size = size
	return data, nil
}
