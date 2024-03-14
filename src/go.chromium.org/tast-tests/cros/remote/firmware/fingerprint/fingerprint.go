// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements functions to interact with the DUT's fingerprint MCU.

package fingerprint

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

type firmwareMetadata struct {
	sha256sum string
	roVersion string
	rwVersion string
	keyID     string
}

type fwInfoType int

const (
	// Elements of firmware information.
	fwInfoTypeSha256sum fwInfoType = iota
	fwInfoTypeRoVersion
	fwInfoTypeRwVersion
	fwInfoTypeKeyID
)

// KeyType The type of key used to sign the firmware.
type KeyType string

// Possible key types.
const (
	KeyTypeDev   KeyType = "dev"
	KeyTypePreMp KeyType = "premp"
	KeyTypeMp    KeyType = "mp"
)

// FirmwareFile describes the firmware filepath and the signing key type.
type FirmwareFile struct {
	FilePath  string
	KeyType   KeyType
	ROVersion string
	RWVersion string
}

// NewFirmwareFile allows for creation of new FirmwareFile struct.
func NewFirmwareFile(filePath string, keyType KeyType, ROVersion, RWVersion string) *FirmwareFile {
	return &FirmwareFile{FilePath: filePath, KeyType: keyType, ROVersion: ROVersion, RWVersion: RWVersion}
}

// NewMPFirmwareFile creates firmwareFile struct for MP-signed image.
func NewMPFirmwareFile(ctx context.Context, d *rpcdut.RPCDUT) (*FirmwareFile, error) {
	fpBoard, err := Board(ctx, d)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get fingerprint board")
	}
	mpFirmwarePath, err := FirmwarePath(ctx, d, fpBoard)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get build fw filepath")
	}
	RWVersion, err := GetBuildRWFirmwareVersion(ctx, d, mpFirmwarePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get expected RW version")
	}
	ROVersion, err := getExpectedFwInfo(fpBoard, mpFirmwarePath, fwInfoTypeRoVersion)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get expected RO version")
	}
	return &FirmwareFile{FilePath: mpFirmwarePath, KeyType: KeyTypeMp, ROVersion: ROVersion, RWVersion: RWVersion}, nil
}

const (
	// WaitForBiodToStartTimeout is the time to wait for biod to start.
	WaitForBiodToStartTimeout = 30 * time.Second
	// timeForCleanup is the amount of time to reserve for cleaning up firmware tests.
	timeForCleanup       = 2 * time.Minute
	flashFpMcuTimeout    = 90 * time.Second
	biodUpstartJobName   = "biod"
	powerdUpstartJobName = "powerd"
	disableFpUpdaterPath = "/var/lib/bio_fw_updater/.disable_fp_updater"
	dutTempPathPattern   = "fp_test_*"
)

// Map from signing key ID to type of signing key.
var keyIDMap = map[string]KeyType{
	// bloonchipper.
	"61382804da86b4156d666cc9a976088f8b647d44": KeyTypeDev,
	"07b1af57220c196e363e68d73a5966047c77011e": KeyTypePreMp,
	"1c590ef36399f6a2b2ef87079c135b69ef89eb60": KeyTypeMp,

	// dartmonkey.
	"257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799": KeyTypeMp,

	// nocturne.
	"8a8fc039a9463271995392f079b83ce33832d07d": KeyTypeDev,
	"6f38c866182bd9bf7a4462c06ac04fa6a0074351": KeyTypeMp,
	"f6f7d96c48bd154dbae7e3fe3a3b4c6268a10934": KeyTypePreMp,

	// nami.
	"754aea623d69975a22998f7b97315dd53115d723": KeyTypePreMp,
	"35486c0090ca390408f1fbbf2a182966084fe2f8": KeyTypeMp,

	// helipilot.
	"ff60ba1fe2cf13f60d0debfb350f7c321115e59a": KeyTypePreMp,
}

// Map of attributes for a given board's various firmware file releases.
// Two purposes:
//  1. Documents the exact versions and keys used for a given firmware file.
//  2. Used to verify that files that end up in the build (and therefore
//     what we release) is exactly what we expect.
var firmwareVersionMap = map[fp.BoardName]map[string]firmwareMetadata{
	fp.BoardNameBloonchipper: {
		"bloonchipper_v2.0.4277-9f652bb3-RO_v2.0.23521-642195b-RW.bin": {
			sha256sum: "f471b70b825e4281461d7d4d29836c9805b8467111a8dcb33ae57c0fd452b22a",
			roVersion: "bloonchipper_v2.0.4277-9f652bb3",
			rwVersion: "bloonchipper_v2.0.23521-642195b",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
		"bloonchipper_v2.0.5938-197506c1-RO_v2.0.23521-642195b-RW.bin": {
			sha256sum: "b95121eecf05e3e7cdcab6ff5b66afbb793b1cc132cf5182af8c2ab4fe230e73",
			roVersion: "bloonchipper_v2.0.5938-197506c1",
			rwVersion: "bloonchipper_v2.0.23521-642195b",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
	},
	fp.BoardNameNocturne: {
		"nocturne_fp_v2.2.64-58cf5974e-RO_v2.0.23516-f14ac415-RW.bin": {
			sha256sum: "ff46c1a423511344043ddd5ab12abf58d844f3eb3d84562efd1bedb661a889a9",
			roVersion: "nocturne_fp_v2.2.64-58cf5974e",
			rwVersion: "nocturne_fp_v2.0.23516-f14ac415",
			keyID:     "6f38c866182bd9bf7a4462c06ac04fa6a0074351",
		},
	},
	fp.BoardNameNami: {
		"nami_fp_v2.2.144-7a08e07eb-RO_v2.0.23516-f14ac4155d-RW.bin": {
			sha256sum: "b20d80b73516c42c986a9991744a55abbf5ccaf830253e517ff7532ba53135da",
			roVersion: "nami_fp_v2.2.144-7a08e07eb",
			rwVersion: "nami_fp_v2.0.23516-f14ac4155d",
			keyID:     "35486c0090ca390408f1fbbf2a182966084fe2f8",
		},
	},
	fp.BoardNameDartmonkey: {
		"dartmonkey_v2.0.2887-311310808-RO_v2.0.23516-f14ac4155-RW.bin": {
			sha256sum: "69eb6282df93eb970e110ea957a8c3b1a951fff3c856c15b4c592dc667201e5e",
			roVersion: "dartmonkey_v2.0.2887-311310808",
			rwVersion: "dartmonkey_v2.0.23516-f14ac4155",
			keyID:     "257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799",
		},
	},
	fp.BoardNameHelipilot: {
		"helipilot_v2.0.24242-633432111f.bin": {
			sha256sum: "ddb5473d432b7ad102a5ace20f2bb043fe528d63336066af82326bddc6babd25",
			roVersion: "helipilot_v2.0.24242-633432111f",
			rwVersion: "helipilot_v2.0.24242-633432111f",
			keyID:     "ff60ba1fe2cf13f60d0debfb350f7c321115e59a",
		},
	},
}

// BoardTransportIsUART returns true if the device communicates with the FPMCU
// using a UART transport.
func BoardTransportIsUART(ctx context.Context, d *rpcdut.RPCDUT) (bool, error) {
	// To compare with CHROMEOS_RELEASE_BOARD in /etc/lsb-release.
	var uartBoards = []string{"guybrush", "zork", "skyrim"}
	hostBoard, err := reporters.New(d.DUT()).Board(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to query host board")
	}
	baseBoard := strings.Split(hostBoard, "-")
	for i := range uartBoards {
		if baseBoard[0] == uartBoards[i] {
			return true, nil
		}
	}
	return false, nil
}

// NeedsRebootAfterFlashing returns true if device needs to be rebooted after flashing.
// For instance, devices using UART transport cannot rebind cros-ec-uart after flashing,
// so an AP reboot is needed to talk to FPMCU. See b/170213489.
func NeedsRebootAfterFlashing(ctx context.Context, d *rpcdut.RPCDUT) (bool, error) {
	return BoardTransportIsUART(ctx, d)
}

func getDUTModel(ctx context.Context, d *rpcdut.RPCDUT) (string, error) {
	// Model comes from "cros_config / name"
	model, err := reporters.New(d.DUT()).Model(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to query DUT model")
	}
	return model, nil
}

// DUTModelIsInList returns true if DUT model is present in the modelList.
func DUTModelIsInList(ctx context.Context, d *rpcdut.RPCDUT, modelList []string) (bool, error) {
	actualModel, err := getDUTModel(ctx, d)
	if err != nil {
		return false, err
	}
	for _, model := range modelList {
		if model == actualModel {
			return true, nil
		}
	}
	return false, nil
}

// getExpectedFwInfo returns expected firmware info for a given firmware file name.
func getExpectedFwInfo(fpBoard fp.BoardName, buildFwFile string, infoType fwInfoType) (string, error) {
	boardExpectedFwInfo, ok := firmwareVersionMap[fpBoard]
	if !ok {
		return "", errors.Errorf("failed to get firmware info for board %s", fpBoard)
	}
	expectedFwInfo, ok := boardExpectedFwInfo[filepath.Base(buildFwFile)]
	if !ok {
		return "", errors.Errorf("failed to get firmware info for file %s", buildFwFile)
	}
	switch infoType {
	case fwInfoTypeSha256sum:
		return expectedFwInfo.sha256sum, nil
	case fwInfoTypeRwVersion:
		return expectedFwInfo.rwVersion, nil
	case fwInfoTypeRoVersion:
		return expectedFwInfo.roVersion, nil
	case fwInfoTypeKeyID:
		return expectedFwInfo.keyID, nil
	default:
		return "", errors.Errorf("failed to get firmware info type %d", infoType)
	}
}

// ValidateBuildFwFile checks that all attributes in the given firmware file match their expected values.
func ValidateBuildFwFile(ctx context.Context, d *rpcdut.RPCDUT, fpBoard fp.BoardName, buildFwFile string) error {
	// Check hash on device.
	actualHash, err := calculateSha256sum(ctx, d, buildFwFile)
	if err != nil {
		return err
	}
	expectedHash, err := getExpectedFwInfo(fpBoard, buildFwFile, fwInfoTypeSha256sum)
	if err != nil {
		return err
	}
	if actualHash != expectedHash {
		return errors.Errorf("failed to validate the sha256 sum, got %s, want %s", actualHash, expectedHash)
	}

	// Check signing key ID.
	actualKeyID, err := readFirmwareKeyID(ctx, d, buildFwFile)
	if err != nil {
		return err
	}
	expectedKeyID, err := getExpectedFwInfo(fpBoard, buildFwFile, fwInfoTypeKeyID)
	if err != nil {
		return err
	}
	if actualKeyID != expectedKeyID {
		return errors.Errorf("failed to validate the key id, got %s, want %s", actualKeyID, expectedKeyID)
	}

	// Check the signing key type is allowed.
	KeyType, ok := keyIDMap[actualKeyID]
	if !ok {
		return errors.Errorf("failed to get key type for key id: %s", actualKeyID)
	}
	if KeyType != KeyTypePreMp && KeyType != KeyTypeMp {
		return errors.Errorf("key type %s is not allowed", KeyType)
	}

	// Check RO version.
	actualRoVersion, err := GetBuildROFirmwareVersion(ctx, d, buildFwFile)
	if err != nil {
		return err
	}
	expectedRoVersion, err := getExpectedFwInfo(fpBoard, buildFwFile, fwInfoTypeRoVersion)
	if err != nil {
		return err
	}
	if actualRoVersion != expectedRoVersion {
		return errors.Errorf("failed to validate the RO version, got %s, want %s", actualRoVersion, expectedRoVersion)
	}

	// Check RW version.
	actualRwVersion, err := GetBuildRWFirmwareVersion(ctx, d, buildFwFile)
	if err != nil {
		return err
	}
	expectedRwVersion, err := getExpectedFwInfo(fpBoard, buildFwFile, fwInfoTypeRwVersion)
	if err != nil {
		return err
	}
	if actualRwVersion != expectedRwVersion {
		return errors.Errorf("failed to validate the RW version, got %s, want %s", actualRwVersion, expectedRwVersion)
	}

	testing.ContextLog(ctx, "Succeeded validating build firmware metadata")
	return nil
}

// GetBuildRWFirmwareVersion returns the RW version of a given build firmware file on DUT.
func GetBuildRWFirmwareVersion(ctx context.Context, d *rpcdut.RPCDUT, buildFWFile string) (string, error) {
	return readFmapSection(ctx, d, buildFWFile, "RW_FWID")
}

// GetBuildROFirmwareVersion returns the RO version of a given build firmware file on DUT.
func GetBuildROFirmwareVersion(ctx context.Context, d *rpcdut.RPCDUT, buildFWFile string) (string, error) {
	return readFmapSection(ctx, d, buildFWFile, "RO_FRID")
}

// readFmapSection reads a section (e.g. RO_FRID) from a firmware file on device.
func readFmapSection(ctx context.Context, d *rpcdut.RPCDUT, buildFwFile, section string) (s string, e error) {
	fs := dutfs.NewClient(d.RPC().Conn)
	// Prepare a temporary file because dump_map only writes the
	// value read from a section to a file (will not just print it to
	// stdout).
	tempdirPath, err := fs.TempDir(ctx, "", "fingerprint_dump_fmap_*")
	if err != nil {
		return "", errors.Wrap(err, "failed to create remote temp directory")
	}
	defer func() {
		if err := fs.RemoveAll(ctx, tempdirPath); err != nil {
			e = errors.Wrapf(err, "failed to remove temp directory: %q", tempdirPath)
		}
	}()

	outputPath := filepath.Join(tempdirPath, section)
	if err := d.Conn().CommandContext(ctx, "dump_fmap", "-x", buildFwFile, fmt.Sprintf("%s:%s", section, outputPath)).Run(ssh.DumpLogOnError); err != nil {
		return "", errors.Wrap(err, "failed to run dump_fmap")
	}

	out, err := d.Conn().CommandContext(ctx, "cat", outputPath).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to read dump_fmap output")
	}
	// dump_fmap writes NULL characters at the end.
	return strings.Trim(string(out), "\x00"), nil
}

// readFirmwareKeyID reads the key id of a firmware file on device.
func readFirmwareKeyID(ctx context.Context, d *rpcdut.RPCDUT, buildFwFile string) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "futility", "show", buildFwFile).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to run futility on device")
	}
	parsed := parseColonDelimitedOutput(string(out))
	keyID, ok := parsed["ID"]
	if !ok {
		return "", errors.Errorf("failed to find key ID for %s", buildFwFile)
	}
	return keyID, nil
}

// calculateSha256sum calculates the sha256sum of a file on device.
func calculateSha256sum(ctx context.Context, d *rpcdut.RPCDUT, buildFwFile string) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "sha256sum", buildFwFile).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to calculate sha256sum on device")
	}
	return strings.Split(string(out), " ")[0], nil
}

// Board returns the name of the fingerprint EC on the DUT
func Board(ctx context.Context, d *rpcdut.RPCDUT) (fp.BoardName, error) {
	// Use cros_config to get fingerprint board.
	out, err := d.Conn().CommandContext(ctx, "cros_config", "/fingerprint", "board").Output(ssh.DumpLogOnError)
	return fp.BoardName(out), err
}

// FirmwarePath returns the path to the fingerprint firmware file on device.
func FirmwarePath(ctx context.Context, d *rpcdut.RPCDUT, fpBoard fp.BoardName) (string, error) {
	cmd := "ls " + fp.FirmwareFilePattern(fpBoard)
	out, err := d.Conn().CommandContext(ctx, "bash", "-c", cmd).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", err
	}
	outStr := strings.TrimSpace(string(out))
	if strings.Contains(outStr, "\n") {
		return "", errors.Errorf("found multiple firmware files for %q: %s", fpBoard, outStr)
	}
	return outStr, nil
}

// FlashFirmware flashes the entire FPMCU using test utility flash_fp_mcu.
// This requires having hardware write-protect disabled.
// It will fail if flashing takes more time than flashFpMcuTimeout or there is
// no enough time.
func FlashFirmware(ctx context.Context, d *rpcdut.RPCDUT, fpFirmwarePath string, needsRebootAfterFlashing bool) error {
	fpBoard, err := Board(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get fp board")
	}
	testing.ContextLogf(ctx, "fp board name: %q", fpBoard)

	if ctxutil.DeadlineBefore(ctx, time.Now().Add(flashFpMcuTimeout)) {
		d, _ := ctx.Deadline()
		t := d.Sub(time.Now())
		return errors.Errorf("insufficient time remaining before the context reaches its deadline. Need at least %v, only %v remain", flashFpMcuTimeout, t)
	}

	flashCmd := []string{"flash_fp_mcu", "--noservices", fpFirmwarePath}
	err = func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, flashFpMcuTimeout)
		defer cancel()

		testing.ContextLogf(ctx, "Running command: %s", shutil.EscapeSlice(flashCmd))
		cmd := d.Conn().CommandContext(ctx, flashCmd[0], flashCmd[1:]...)
		out, err := cmd.CombinedOutput()
		testing.ContextLog(ctx, "flash_fp_mcu output:", "\n", string(out))

		return err
	}(ctx)
	if err != nil {
		return errors.Wrap(err, "flash_fp_mcu failed")
	}

	if needsRebootAfterFlashing {
		testing.ContextLog(ctx, "Rebooting")
		if err := d.Reboot(ctx); err != nil {
			return errors.Wrap(err, "rebooting failed")
		}
	}

	return nil
}

// FlashFirmwareUpdate updates one particular firmware region/image of the
// FPMCU using the firmware update mechanism.
// This does not require having hardware write-protect disabled and will not
// clear entropy or software write-protect status.
func FlashFirmwareUpdate(ctx context.Context, d *rpcdut.RPCDUT, image FWImageType, file string) error {
	fs := dutfs.NewClient(d.RPC().Conn)
	exists, err := fs.Exists(ctx, file)
	if err != nil {
		return errors.Wrapf(err, "error checking that file exists: %q", file)
	}
	if !exists {
		return errors.Errorf("file does not exist: %q", file)
	}

	flashCmd := []string{
		"/opt/sbin/crosec-legacy-drv",
		"--noverify-all",             // only verify included regions
		"--verbose",                  // verbose
		"--programmer", "ec:type=fp", // use "programmer" for fingerprint "EC"
		"--include", "EC_" + string(image), // only write the specific image
		"--write", file,
	}
	testing.ContextLogf(ctx, "Running command: %s", shutil.EscapeSlice(flashCmd))
	cmd := d.Conn().CommandContext(ctx, flashCmd[0], flashCmd[1:]...)
	if err := cmd.Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "flashrom failed")
	}

	return nil
}

// InitializeEntropy initializes the anti-rollback block in RO firmware.
func InitializeEntropy(ctx context.Context, d *rpcdut.RPCDUT) error {
	if err := d.Conn().CommandContext(ctx, "bio_wash", "--factory_init").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to initialize entropy")
	}
	return nil
}

// ReimageFPMCU flashes the FPMCU completely and initializes entropy.
func ReimageFPMCU(ctx context.Context, d *rpcdut.RPCDUT, pxy *servo.Proxy, firmwareFile string, needsRebootAfterFlashing bool) error {
	if err := pxy.Servo().SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		return errors.Wrap(err, "failed to disable HW write protect")
	}
	if err := FlashFirmware(ctx, d, firmwareFile, needsRebootAfterFlashing); err != nil {
		return errors.Wrap(err, "failed to flash FP firmware")
	}
	testing.ContextLog(ctx, "Flashed FP firmware, now initializing the entropy")
	if err := InitializeEntropy(ctx, d); err != nil {
		return err
	}
	testing.ContextLog(ctx, "Entropy initialized, now rebooting to get seed")
	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}
	if err := pxy.Servo().SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		return errors.Wrap(err, "failed to enable HW write protect")
	}
	return nil
}

// InitializeKnownState checks that the AP can talk to FPMCU. If not, it flashes the FPMCU. It then checks if SWWP will need to be removed. If so, it reflashes.
func InitializeKnownState(ctx context.Context, d *rpcdut.RPCDUT, outdir string, pxy *servo.Proxy, fpBoard fp.BoardName, firmwareFile FirmwareFile, needsRebootAfterFlashing, removeSWWP bool) error {
	// Check if the FPMCU even responds to a friendly hello (query version).
	// Save the version string in a file for later.
	out, err := CheckFirmwareIsFunctional(ctx, d.DUT())
	if err != nil {
		testing.ContextLogf(ctx, "FPMCU firmware is not functional (error: %v). Reflashing FP firmware", err)
		if err := ReimageFPMCU(ctx, d, pxy, firmwareFile.FilePath, needsRebootAfterFlashing); err != nil {
			return err
		}
	}
	// If we need to remove software write protect, we must reflash here.
	testing.ContextLog(ctx, "Checking if software write protect needs to be removed")
	fp, err := GetFlashProtect(ctx, d.DUT())
	if err != nil {
		return errors.Wrap(err, "failed to read flash protect")
	}
	if removeSWWP && fp.IsSoftwareReadOutProtected() {
		testing.ContextLog(ctx, "Software write protect had previously been enabled. Reflashing FP firmware")
		if err := ReimageFPMCU(ctx, d, pxy, firmwareFile.FilePath, needsRebootAfterFlashing); err != nil {
			return errors.Wrap(err, "failed to remove software write protect")
		}
	}
	versionOutputFile := "cros_fp_version.txt"
	testing.ContextLogf(ctx, "Writing FP firmware version to %s", versionOutputFile)
	if err := ioutil.WriteFile(filepath.Join(outdir, versionOutputFile), out, 0644); err != nil {
		// This is a nonfatal error that shouldn't kill the test.
		testing.ContextLog(ctx, "Failed to write FP firmware version to file: ", err)
	}

	// Check all other standard FPMCU state.
	testing.ContextLog(ctx, "Checking other FPMCU state")
	if err := CheckValidFlashState(ctx, d, fpBoard, firmwareFile); err != nil {
		testing.ContextLogf(ctx, "%v. Reflashing FP firmware", err)
		if err := ReimageFPMCU(ctx, d, pxy, firmwareFile.FilePath, needsRebootAfterFlashing); err != nil {
			return err
		}
	}

	return nil
}

// CheckValidFlashState validates the rollback state and the running firmware versions (RW and RO).
// It returns an error if any of the values are incorrect.
func CheckValidFlashState(ctx context.Context, d *rpcdut.RPCDUT, fpBoard fp.BoardName, firmwareFile FirmwareFile) error {
	// Check that RO and RW versions are what we expect.
	if err := CheckRunningFirmwareVersionMatches(ctx, d, firmwareFile.ROVersion, firmwareFile.RWVersion); err != nil {
		return err
	}

	// Similar to bio_fw_updater, check is the active FW copy is RW. If it isn't
	// that might mean that there is a firmware issue.
	if err := CheckRunningFirmwareCopy(ctx, d.DUT(), ImageTypeRW); err != nil {
		return errors.Wrap(err, "FPMCU is not in RW")
	}

	// Check that no tests enabled anti-rollback and that entropy has been added
	// (maybe multiple times).
	rollback, err := RollbackInfo(ctx, d.DUT())
	if err != nil {
		return errors.Wrap(err, "failed to retrieve rollbackinfo")
	}
	if rollback.IsAntiRollbackSet() {
		return errors.Wrap(err, "FPMCU has anti-rollback enabled")
	}
	// This might be considered overkill to claim the FPMCU is not in a valid
	// state if entropy is not set. The reason we are doing this is so that
	// the caller will invoke ReimageFPMCU, which has a known good sequence to
	// add entropy (and reboot dut...).
	if !rollback.IsEntropySet() {
		return errors.Wrap(err, "FPMCU doesn't have entropy set")
	}

	return nil
}

// InitializeHWAndSWWriteProtect ensures hardware and software write protect are initialized as requested.
func InitializeHWAndSWWriteProtect(ctx context.Context, d *rpcdut.RPCDUT, pxy *servo.Proxy, fpBoard fp.BoardName, enableHWWP, enableSWWP bool) error {
	testing.ContextLogf(ctx, "Initializing HW WP to %t, SW WP to %t", enableHWWP, enableSWWP)
	// The HW write protect level must match the desired SW write protect
	// level prior to modifying SW write protect. Once the SW write protect
	// level has been updated it is safe to set the HW write protect to the
	// desired level.
	// Write protect truth table:
	// sw: 0, hw: 0 => hw(0) -> sw(0)
	// sw: 0, hw: 1 => hw(0) -> sw(0) -> hw(1)
	// sw: 1, hw: 0 => hw(1) -> sw(1) -> hw(0)
	// sw: 1, hw: 1 => hw(1) -> sw(1)
	if err := SetHardwareWriteProtect(ctx, pxy, enableSWWP); err != nil {
		return err
	}

	if err := SetSoftwareWriteProtect(ctx, d.DUT(), enableSWWP); err != nil {
		return err
	}

	if enableHWWP != enableSWWP {
		// Set HW write protect to target state.
		if err := SetHardwareWriteProtect(ctx, pxy, enableHWWP); err != nil {
			return err
		}
	}

	if err := CheckWriteProtectStateCorrect(ctx, d.DUT(), enableSWWP, enableHWWP); err != nil {
		return errors.Wrap(err, "failed to validate write protect settings")
	}

	return nil
}

// RunningRWVersion returns the RW version running on FPMCU.
func RunningRWVersion(ctx context.Context, d *rpcdut.RPCDUT) (string, error) {
	return runningFirmwareVersion(ctx, d.DUT(), ImageTypeRW)
}

// RunningROVersion returns the RO version running on FPMCU.
func RunningROVersion(ctx context.Context, d *rpcdut.RPCDUT) (string, error) {
	return runningFirmwareVersion(ctx, d.DUT(), ImageTypeRO)
}

// CheckRunningFirmwareVersionMatches compares the running RO and RW firmware
// versions to expectedROVersion and expectedRWVersion and returns an error if
// they do not match.
func CheckRunningFirmwareVersionMatches(ctx context.Context, d *rpcdut.RPCDUT, expectedROVersion, expectedRWVersion string) error {
	runningRWVersion, err := RunningRWVersion(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get RW version")
	}

	runningROVersion, err := RunningROVersion(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get RO version")
	}

	if runningRWVersion != expectedRWVersion {
		return errors.Errorf("failed to validate the RW firmware version: running %q, expected %q", runningRWVersion, expectedRWVersion)
	}

	if runningROVersion != expectedROVersion {
		return errors.Errorf("failed to validate the RO firmware version: running %q, expected %q", runningROVersion, expectedROVersion)
	}

	return nil
}

// CheckRollbackState checks that the anti-rollback block is set to expected values.
func CheckRollbackState(ctx context.Context, d *rpcdut.RPCDUT, expected RollbackState) error {
	actual, err := RollbackInfo(ctx, d.DUT())
	if err != nil {
		return err
	}

	if actual != expected {
		return errors.Errorf("Rollback not set correctly, expected: %q, actual: %q", expected, actual)
	}

	return nil
}

// BioWash calls bio_wash to reset the entropy key material on the FPMCU.
func BioWash(ctx context.Context, d *rpcdut.RPCDUT, reset bool) error {
	cmd := []string{"bio_wash"}
	if !reset {
		cmd = append(cmd, "--factory_init")
	}
	return d.Conn().CommandContext(ctx, cmd[0], cmd[1:]...).Run()
}

// CheckRawFPFrameFails validates that a raw frame cannot be read from the FPMCU
// and returns an error if a raw frame can be read.
func CheckRawFPFrameFails(ctx context.Context, d *rpcdut.RPCDUT) error {
	const fpFrameRawAccessDeniedError = `EC result 4 (ACCESS_DENIED)
Failed to get FP sensor frame
`
	const fpFrameRawAccessDeniedError2 = `ioctl -1, errno 13 (Permission denied), EC result 255 (<unknown>)
ioctl -1, errno 13 (Permission denied), EC result 255 (<unknown>)
ioctl -1, errno 13 (Permission denied), EC result 255 (<unknown>)
Failed to get FP sensor frame
`
	var stderrBuf bytes.Buffer

	cmd := rawFPFrameCommand(ctx, d.DUT())
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err == nil {
		return errors.New("command to read raw frame succeeded")
	}

	stderr := string(stderrBuf.Bytes())
	if stderr != fpFrameRawAccessDeniedError && stderr != fpFrameRawAccessDeniedError2 {

		return errors.Errorf("raw fpframe command returned unexpected value, expected1: %q, expected2: %q, actual: %q", fpFrameRawAccessDeniedError, fpFrameRawAccessDeniedError2, stderr)
	}

	return nil
}
