// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	defaultUpdater       = "/usr/sbin/chromeos-firmwareupdate"
	meRwVersionFilename  = "me_rw.version"
	meRwMetadataFilename = "me_rw.metadata"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CsmeFwUpdate,
		Desc:         "Verifies that CSME RW firmware can be upgraded or downgraded using chromeos-firmwareupdate --mode=recovery",
		Contacts:     []string{"digehlot@google.com", "chromeos-firmware@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
		SoftwareDeps: []string{"csme_update"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Vars:         []string{"firmware_branch", "ro_versions"},
		Data:         []string{"shipped-firmwares.json"},
		Timeout:      40 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Val:     fixture.NormalMode,
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
			},
			{
				Name:    "dev",
				Val:     fixture.DevModeGBB,
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevModeGBB),
			},
		},
	})
}

// CsmeFwUpdate tests csme rw firmware update feature by changing the me_rw
// image in firmware main regions with a different version
func CsmeFwUpdate(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	h := s.FixtValue().(*fixture.Value).Helper

	tempdir, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp/", "-t", "CSME_XXXXXXXX").Output()
	if err != nil {
		s.Fatal("Failed to create remote data path directory: ", err)
	}
	tempDirOnDut := strings.TrimSpace(string(tempdir))
	defer func() {
		s.Log("Delete temporary test home directory and contained files from DUT")
		if _, err := h.DUT.Conn().CommandContext(cleanupCtx, "rm", "-rf", tempDirOnDut).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete test home directory: ", err)
		}
	}()

	backupOnDut := filepath.Join(tempDirOnDut, "bios_original.bin")
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, backupOnDut); err != nil {
		s.Fatal("Failed to send AP firmware backup to DUT: ", err)
	}

	fwName, err := getFwName(ctx, h.Reporter)
	if err != nil {
		s.Fatal("Failed to get firmware name: ", err)
	}

	downgradeBiosImageOnDut, err := getDowngradeBiosImage(ctx, h.DUT, fwName, tempDirOnDut)
	if err != nil {
		s.Fatal("Failed to get downgrade image: ", err)
	}

	if err := compareFmapScheme(ctx, h.DUT, backupOnDut, downgradeBiosImageOnDut); err != nil {
		s.Fatal("FMap comparison failure: ", err)
	}

	originalMeVersion, downgradeMeVersion, isDowngradePossible, err := getCsmeVersions(ctx, h.DUT, tempDirOnDut, backupOnDut, downgradeBiosImageOnDut)
	if err != nil {
		s.Fatal("Failed to get CSME versions: ", err)
	}
	if !isDowngradePossible {
		s.Log("WARNING! CSME RW blobs are same in downgrade and original bios")

		// Create a new directory in servo host to store the downloaded files.
		out, err := h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "CsmeFwUpdateXXXXXX")
		if err != nil {
			s.Fatal("Failed to create servo temp dir")
		}
		tmpDirServo := strings.TrimSuffix(string(out), "\n")
		defer func() {
			s.Log("Deleting tmp directory on servo: ", tmpDirServo)
			if err := h.ServoProxy.RunCommand(cleanupCtx, false, "rm", "-rf", tmpDirServo); err != nil {
				s.Fatal("Failed to delete temp directory on servo host: ", err)
			}
		}()

		// Replace downgrade bios with shipped bios
		downgradeBiosImageOnDut, err := flashLatestShippedFirmwareInRO(ctx, s, tempDirOnDut, tmpDirServo, backupOnDut)
		if err != nil {
			s.Fatal("CSME RW blobs are same in downgrade and original bios while failed to get shipped bios : ", err)
		}
		originalMeVersion, downgradeMeVersion, isDowngradePossible, err = getCsmeVersions(ctx, h.DUT, tempDirOnDut, backupOnDut, downgradeBiosImageOnDut)
		if err != nil {
			s.Fatal("Failed to get CSME versions: ", err)
		}
		if !isDowngradePossible {
			s.Fatal("CSME RW blobs are same in downgrade, original and shipped bios")
		}
	}

	// Reboot before reading CBMEM log
	s.Log("Reboot and wait for DUT to reconnect")
	if err := h.DUT.Reboot(ctx); err != nil {
		s.Fatal("failed to reboot DUT", err)
	}
	activeMeVersion, err := getActiveCsmeRwVersion(ctx, h.DUT)
	if err != nil {
		s.Fatal("Failed to get active ME version: ", err)
	} else if activeMeVersion != originalMeVersion {
		s.Fatalf("Incorrect DUT state. Cannot start the test. Expected ME version is %q, got %q", originalMeVersion, activeMeVersion)
	}
	s.Logf("Active CSME RW Version: %s", activeMeVersion)

	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		s.Fatal("Failed to setup futility instance: ", err)
	}

	recoveryRequired := false
	defer func() {
		if !recoveryRequired {
			return
		}

		originalOpts := futility.NewUpdateOptions(backupOnDut).WithMode(futility.UpdateModeRecovery).WithWriteProtection(futility.WriteProtectionEnable)
		if out, err := futilityInstance.Update(cleanupCtx, originalOpts); err != nil {
			s.Fatal("Failed to restore original firmware image: ", err, "\nOutput:\n", string(out))
		}
	}()

	for _, slot := range []bios.ImageSection{bios.FWBodyAImageSection, bios.FWBodyBImageSection} {
		recoveryRequired = true
		s.Log("Downgrading RW section. Downgrade ME Version: ", downgradeMeVersion)
		downgradeOpts := futility.NewUpdateOptions(downgradeBiosImageOnDut).WithMode(futility.UpdateModeRecovery).WithWriteProtection(futility.WriteProtectionEnable)
		if out, err := futilityInstance.Update(ctx, downgradeOpts); err != nil {
			s.Fatal("Failed to flash downgraded firmware image: ", err, "\nOutput:\n", string(out))
		}

		if err := switchSlotAndVerifyCsme(ctx, h.DUT, h.Reporter, slot, downgradeMeVersion); err != nil {
			s.Fatal("Failed to switch to downgraded ME: ", err)
		}

		s.Log("Upgrading RW section. Updrade ME Version: ", originalMeVersion)
		originalOpts := futility.NewUpdateOptions(backupOnDut).WithMode(futility.UpdateModeRecovery).WithWriteProtection(futility.WriteProtectionEnable)
		if out, err := futilityInstance.Update(ctx, originalOpts); err != nil {
			s.Fatal("Failed to flash original firmware image: ", err, "\nOutput:\n", string(out))
		}

		if err := switchSlotAndVerifyCsme(ctx, h.DUT, h.Reporter, slot, originalMeVersion); err != nil {
			s.Fatal("Failed to switch to original ME: ", err)
		}
		recoveryRequired = false
	}
}

func getFwName(ctx context.Context, reporter *reporters.Reporter) (string, error) {
	fwName, err := reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		return "", errors.Wrap(err, "cannot obtain FWID from crossystem params")
	}
	re := regexp.MustCompile(`Google_([a-z-A-Z-0-9]*)\.(\d*)\.\d*.\d*`)
	match := re.FindStringSubmatch(fwName)
	if len(match) != 3 {
		return "", errors.Errorf("unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, fwName)
	}
	fwName = strings.ToLower(match[1])
	testing.ContextLog(ctx, "Firmware Version: ", fwName)
	return fwName, nil
}

func getDowngradeBiosImage(ctx context.Context, dut *dut.DUT, fwName, workDir string) (string, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Get relative image path
	chromeosFirmwareUpdateManifest := fmt.Sprintf("chromeos-firmwareupdate --manifest | jq -c .%s.host.image", fwName)
	imagePathBytes, err := dut.Conn().CommandContext(ctx, "bash", "-c", chromeosFirmwareUpdateManifest).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "chromeos-firmwareupdate --manifest read failed. Output: %v", string(imagePathBytes))
	}

	// Unpack image to DUT temporary directory
	shellballDir := filepath.Join(workDir, "shellball")
	shellballBios := filepath.Join(shellballDir, strings.Trim(string(imagePathBytes), "\" \n"))
	if err := dut.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "--unpack", shellballDir).Run(ssh.DumpLogOnError); err != nil {
		return "", errors.Wrap(err, "failed to unpack firmware shellball")
	}
	defer func() {
		// Ignore error. shellballDir should get removed anyway during workDir cleanup.
		dut.Conn().CommandContext(cleanupCtx, "rm", "-r", shellballDir).Run(ssh.DumpLogOnError)
	}()

	// Move Bios to tmp dir
	downgradeBios := filepath.Join(workDir, "bios_downgrade.bin")
	if err := dut.Conn().CommandContext(ctx, "mv", shellballBios, downgradeBios).Run(); err != nil {
		return "", errors.Wrapf(err, "failed to move %s to %s", shellballBios, downgradeBios)
	}

	testing.ContextLog(ctx, "Downgrade BIOS is stored at: ", downgradeBios)
	return downgradeBios, nil
}

func compareFmapScheme(ctx context.Context, dut *dut.DUT, originalBios, downgradeBios string) error {
	futilityInstance, err := futility.NewLocalBuilder(dut).Build()
	if err != nil {
		return errors.Wrap(err, "failed to setup futility instance")
	}

	fmapOriginalBios, out, err := futilityInstance.DumpFmap(ctx, originalBios, []string{"ME_RW_A"})
	if err != nil {
		return errors.Wrapf(err, "failed to run futility dump_fmap. Output: %v", string(out))
	}
	testing.ContextLog(ctx, "Original image ME_RW_A: ", fmapOriginalBios)

	fmapDowngradeBios, out, err := futilityInstance.DumpFmap(ctx, downgradeBios, []string{"ME_RW_A"})
	if err != nil {
		return errors.Wrapf(err, "failed to run futility dump_fmap. Output: %v", string(out))
	}
	testing.ContextLog(ctx, "Downgrade image ME_RW_A: ", fmapDowngradeBios)

	if (len(fmapOriginalBios) == 0) != (len(fmapDowngradeBios) == 0) {
		return errors.New("Test setup issue: FMAP format is different in original and downgrade bios")
	}

	return nil
}

func getCsmeVersions(ctx context.Context, dut *dut.DUT, workDir, originalBios, downgradeBios string) (originalMeVersion, downgradeMeVersion string, downgradePossible bool, err error) {
	originalMeVersion, err = getImageCsmeRwVersion(ctx, dut, workDir, originalBios)
	if err != nil {
		return "", "", false, errors.Wrap(err, "failed to get ME version")
	}

	downgradeMeVersion, err = getImageCsmeRwVersion(ctx, dut, workDir, downgradeBios)
	if err != nil {
		return "", "", false, errors.Wrap(err, "failed to get ME version")
	}

	testing.ContextLogf(ctx, "FW main CSME RW Version original Image : %s", originalMeVersion)
	testing.ContextLogf(ctx, "FW main CSME RW Version downgrade Image: %s", downgradeMeVersion)

	downgradePossible = true
	if originalMeVersion == downgradeMeVersion {
		identical, err := isMeRwBlobsIdentical(ctx, dut, workDir, originalBios, downgradeBios)
		if err != nil {
			return "", "", false, errors.Wrap(err, "failed compare ME blobs")
		}
		downgradePossible = !identical
	}

	return // All values filled before
}

// getImageCsmeRwVersion extracts the ME RW version from the given firmware image. Newer firmware
// stores the version in a CBFS file called me_rw.version. Older firmware stores it in
// me_rw.metadata, which contains both the version and a hash. Check which of these is present,
// extract the version from it, and return it as a string, e.g. "13.50.15.1521".
func getImageCsmeRwVersion(ctx context.Context, dut *dut.DUT, workDir, binPath string) (string, error) {
	// List CBFS files using cbfstool.
	out, err := dut.Conn().CommandContext(ctx, "cbfstool", binPath, "print", "-r", string(bios.FWBodyAImageSection)).Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to list CBFS files with cbfstool of section %v from %v", bios.FWBodyAImageSection, binPath)
	}
	outs := string(out)

	// Check of which of me_rw.version and me_rw.metadata is present.
	var versionFilename string
	hasMeRwVersion := strings.Contains(outs, meRwVersionFilename)
	hasMeRwMetadata := strings.Contains(outs, meRwMetadataFilename)
	if hasMeRwVersion && hasMeRwMetadata {
		return "", errors.Errorf("image contains both %s and %s", meRwVersionFilename, meRwMetadataFilename)
	} else if !hasMeRwVersion && !hasMeRwMetadata {
		return "", errors.Errorf("image contains neither %s nor %s", meRwVersionFilename, meRwMetadataFilename)
	} else if hasMeRwVersion {
		versionFilename = meRwVersionFilename
	} else {
		versionFilename = meRwMetadataFilename
	}
	testing.ContextLogf(ctx, "Getting ME RW version for %q from %s", binPath, meRwVersionFilename)

	// Extract the file from CBFS and read its contents as a byte array.
	file := filepath.Join(workDir, "me_rw_version.bin")
	if err := cbfsRead(ctx, dut, binPath, string(bios.FWBodyAImageSection), versionFilename, file); err != nil {
		return "", errors.Wrapf(err, "failed to extract %v from section %v of %v", versionFilename, bios.FWBodyAImageSection, binPath)
	}

	bytes, err := linuxssh.ReadFile(ctx, dut.Conn(), file)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read ME RW version file %q from DUT", file)
	}

	// Extract the version as a string.
	if hasMeRwVersion {
		// me_rw.version just contains the version as a string.
		return strings.TrimSpace(string(bytes)), nil
	}

	// The first 8 bytes of me_rw.metadata contain the version. Each pair of bytes is
	// converted to a decimal int, and they're concatenated with dots in between.
	var nums []string
	for i := 0; i < 4; i++ {
		num := binary.LittleEndian.Uint16(bytes[i*2 : i*2+2])
		nums = append(nums, strconv.FormatUint(uint64(num), 10))
	}
	return strings.Join(nums, "."), nil
}

func cbfsRead(ctx context.Context, dut *dut.DUT, binPath, region, blob, filename string) error {
	extractCmd := fmt.Sprintf("cbfstool %s extract -r %s -n %s -f %s", binPath, region, blob, filename)
	out, err := dut.Conn().CommandContext(ctx, "bash", "-c", extractCmd).Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "cbfstool failed to extract binary: %s", string(out))
	}
	return nil
}

func isMeRwBlobsIdentical(ctx context.Context, dut *dut.DUT, workDir, originalBios, downgradeBios string) (bool, error) {
	downgradeRw := filepath.Join(workDir, "me_rw_a_downgrade.bin")
	originalRwA := filepath.Join(workDir, "me_rw_a_original.bin")
	originalRwB := filepath.Join(workDir, "me_rw_b_original.bin")

	if err := cbfsRead(ctx, dut, downgradeBios, "ME_RW_A", "me_rw", downgradeRw); err != nil {
		return false, errors.Wrapf(err, "failed to read ME_RW_A from %v", downgradeRw)
	}
	if err := cbfsRead(ctx, dut, originalBios, "ME_RW_A", "me_rw", originalRwA); err != nil {
		return false, errors.Wrapf(err, "failed to read ME_RW_A from %v", originalRwA)
	}
	if err := cbfsRead(ctx, dut, originalBios, "ME_RW_B", "me_rw", originalRwB); err != nil {
		return false, errors.Wrapf(err, "failed to read ME_RW_B from %v", originalRwB)
	}

	testing.ContextLog(ctx, "Comparing ME blobs")
	diffA, err := cmpLocalFiles(ctx, dut, downgradeRw, originalRwA)
	if err != nil {
		return false, errors.Wrapf(err, "failed to compare %v with %v", downgradeRw, originalRwA)
	}
	diffB, err := cmpLocalFiles(ctx, dut, downgradeRw, originalRwB)
	if err != nil {
		return false, errors.Wrapf(err, "failed to compare %v with %v", downgradeRw, originalRwB)
	}

	if diffA != "" {
		testing.ContextLog(ctx, "CSME RW version is same, but downgrade image ME_RW_A:me_rw differs from ME_RW_A:me_rw in the original firmware image")
	}
	if diffB != "" {
		testing.ContextLog(ctx, "CSME RW version is same, but downgrade image ME_RW_A:me_rw differs from ME_RW_B:me_rw in the original firmware image")
	}

	return diffA == "" && diffB == "", nil
}

func cmpLocalFiles(ctx context.Context, dut *dut.DUT, file1, file2 string) (string, error) {
	out, err := dut.Conn().CommandContext(ctx, "cmp", file2, file2).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "file comparison failed")
	}
	return string(out), nil
}

func getActiveCsmeRwVersion(ctx context.Context, dut *dut.DUT) (string, error) {
	// Get CSE version from coreboot log.
	const cbmemCommand = "cbmem -1 | grep cse_lite:"
	corebootLog, err := dut.Conn().CommandContext(ctx, "bash", "-c", cbmemCommand).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to extract coreboot logs")
	}

	// Parse CSME string in coreboot log.
	re := regexp.MustCompile(`cse_lite: RW version = ([0-9\.]+)`)
	match := re.FindStringSubmatch(string(corebootLog))
	csmeVersion := ""
	if len(match) > 1 {
		csmeVersion = match[1]
	}
	return csmeVersion, nil
}

func switchSlotAndVerifyCsme(ctx context.Context, dut *dut.DUT, reporter *reporters.Reporter, slot bios.ImageSection, expectedMe string) error {
	testing.ContextLogf(ctx, "Switching to %s", slot)
	slotShort := "A"
	if slot == bios.FWBodyBImageSection {
		slotShort = "B"
	}

	if err := reporter.CrossystemSetParam(ctx, reporters.CrossystemParamFWTryNext, slotShort); err != nil {
		return errors.Wrap(err, "failed to set crossystem fw_try_next")
	}

	testing.ContextLog(ctx, "Reboot and wait for DUT to reconnect")
	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	if value, err := reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct); err != nil {
		return errors.Wrap(err, "failed to get active firmware slot")
	} else if value != slotShort {
		return errors.Errorf("failed to switch to requested slot. Expected %q but got %q", slotShort, value)
	}

	activeMeVersion, err := getActiveCsmeRwVersion(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get active ME version")
	}
	testing.ContextLogf(ctx, "Active CSME RW Version after switch: %s", activeMeVersion)

	if activeMeVersion != expectedMe {
		return errors.Errorf("CSME RW switch to %s failed. Expected ME version is %q, got %q", slot, expectedMe, activeMeVersion)
	}

	testing.ContextLogf(ctx, "Switch to slot %s successful", slot)
	return nil
}

func flashLatestShippedFirmwareInRO(ctx context.Context, s *testing.State, tempDirOnDut string, tmpDirServo string, originalBios string) (string, error) {
	h := s.FixtValue().(*fixture.Value).Helper

	// Create a new directory to store the downloaded files.
	tmpDir, err := os.MkdirTemp("", "firmware-CsmeFwUpdate")
	if err != nil {
		s.Fatal("Failed to create a new directory for the test: ", err)
	}
	defer os.RemoveAll(tmpDir)

	// Get firmware name
	corebootName, err := getFwName(ctx, h.Reporter)
	if err != nil {
		s.Fatal("Failed to get firmware name: ", err)
	}

	// Shipped version Data
	var shippedFwVersions []jsonFwInfo
	fwBranchVar, fwBranchVarOk := s.Var("firmware_branch")
	roVersions, roVersionsOk := s.Var("ro_versions")
	if roVersionsOk && fwBranchVarOk {
		for _, ro := range strings.Split(roVersions, ",") {
			shippedFwVersions = append(shippedFwVersions, jsonFwInfo{
				Board:  h.Board,
				Model:  h.Model,
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
			s.Fatal("While collecting the shipped fw versions details ", err)
			return "", errors.Wrapf(err, "failed while collecting the shipped fw versions;")
		}
		s.Logf("SHIPPED firmwares found for model %s:", h.Model)
	}

	// Download the latest shipped firmware.
	firmwareFilesToFlash, err := downloadAndUntarFwFile(ctx, s, h, tmpDir, tmpDirServo, corebootName, shippedFwVersions[len(shippedFwVersions)-1])
	if err != nil {
		return "", errors.Wrapf(err, "failed while downloading the shipped fw versions;")
	}
	biosImageServo := filepath.Join(tmpDirServo, firmwareFilesToFlash.APFirmwareFile)
	biosImageOnDut := tempDirOnDut + "/bios_downgrade.bin"

	// Copy shipped firmware to DUT
	aMAP := make(map[string]string)
	aMAP[biosImageServo] = biosImageOnDut
	_, err = linuxssh.PutFiles(ctx, s.DUT().Conn(), aMAP, linuxssh.DereferenceSymlinks)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to copy image firmware from Host to the Dut;")
	}
	s.Logf("Downloaded AP Firmware %s", biosImageOnDut)

	downgradeMeVersion, err := getImageCsmeRwVersion(ctx, h.DUT, tempDirOnDut, biosImageOnDut)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get ME version of shipped image;")
	}
	s.Logf("FW main CSME RW Version Shipped Image: %s", downgradeMeVersion)

	s.Log("Flashing shipped RO, Shipped RW")
	if err := h.RequireBiosServiceClient(ctx); err != nil {
		return "", errors.Wrapf(err, "failed to setup BiosServiceClient;")
	}
	flashingCtx, cancelflashingCtx := context.WithTimeout(ctx, flashingTime)
	defer cancelflashingCtx()
	s.Logf("Flashing DUT with file: %s using section: %v programmer: %v", biosImageOnDut, fwpb.ImageSection_EmptyImageSection, fwpb.Programmer_BIOSProgrammer)
	if _, err := h.BiosServiceClient.WriteImageFromMultiSectionFile(flashingCtx, &fwpb.FWSectionInfo{Programmer: fwpb.Programmer_BIOSProgrammer, Path: biosImageOnDut, Section: fwpb.ImageSection_EmptyImageSection}); err != nil {
		return "", errors.Wrapf(err, "failed to flash DUT with the Shipped bin image;")
	}

	s.Log("Flashing current RW A")
	if _, err := h.BiosServiceClient.WriteImageFromMultiSectionFile(flashingCtx, &fwpb.FWSectionInfo{Programmer: fwpb.Programmer_BIOSProgrammer, Path: originalBios, Section: fwpb.ImageSection_APRWAImageSection}); err != nil {
		return "", errors.Wrapf(err, "failed to flash DUT RW_A with the current bin image;")
	}

	s.Log("Flashing current RW B")
	if _, err := h.BiosServiceClient.WriteImageFromMultiSectionFile(flashingCtx, &fwpb.FWSectionInfo{Programmer: fwpb.Programmer_BIOSProgrammer, Path: originalBios, Section: fwpb.ImageSection_APRWBImageSection}); err != nil {
		return "", errors.Wrapf(err, "failed to flash DUT RW_B with the current bin image;")
	}

	testing.ContextLog(ctx, "Reboot and wait for DUT to reconnect")
	if err := h.DUT.Reboot(ctx); err != nil {
		return "", errors.Wrapf(err, "failed to reboot DUT post flashing shipped RO and current RW;")
	}

	s.Logf("Restoring temp directory: %s  after reboot", tempDirOnDut)
	// All DUT files has been deleted, so restore
	_, err = h.DUT.Conn().CommandContext(ctx, "mkdir", "-p", tempDirOnDut).Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to create remote data path directory;")
	}

	// Copy Downgrade image.
	s.Logf("Copying downgrade bios to DUT %s", biosImageOnDut)
	_, err = linuxssh.PutFiles(ctx, s.DUT().Conn(), aMAP, linuxssh.DereferenceSymlinks)
	if err != nil {
		return "", errors.Wrapf(err, "failed to copy image firmware from Host to the Dut;")
	}

	// Copy Original image.
	s.Logf("Fetching orginal Bios %s", originalBios)
	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, originalBios); err != nil {
		return "", errors.Wrapf(err, "failed to send AP firmware backup to DUT;")
	}
	return biosImageOnDut, nil
}
