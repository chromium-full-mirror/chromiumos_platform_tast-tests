// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/chromiumos/config/go/api"
	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
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
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
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
	/*
	 * Leverage the firmware feature "CSE force sync" to test the CSE sync operation,
	 * if vboot CBFS integration is enabled.
	 */
	vbootCbfs := s.Features("").Hardware.HardwareFeatures.FwConfig.VbootCbfsIntegration == api.HardwareFeatures_PRESENT
	if vbootCbfs {
		s.Log("Voot CBFS sync is enabled, performing forced cse sync")
		if err := performForcedCseSync(ctx, s); err != nil {
			s.Fatal("Forced CSE sync failed: ", err)
		}
		return
	}

	isDowngradePossible, err := performCseSync(ctx, s)
	if err != nil {
		s.Fatal("CSE sync failed: ", err)
	}
	if !isDowngradePossible {
		s.Log("WARNING! CSME RW blobs are same in downgrade and original bios")
		if err := performForcedCseSync(ctx, s); err != nil {
			s.Fatal("Forced CSE sync failed: ", err)
		}
	}
}
func performCseSync(ctx context.Context, s *testing.State) (bool, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()
	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	h := s.FixtValue().(*fixture.Value).Helper

	tempdir, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp/", "-t", "CSME_XXXXXXXX").Output()
	if err != nil {
		return false, errors.Wrap(err, "failed to create remote data path directory")
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
		return false, errors.Wrap(err, "failed to send AP firmware backup to DUT")
	}

	fwName, err := getFwName(ctx, h.Reporter)
	if err != nil {
		return false, errors.Wrap(err, "failed to get firmware name")
	}

	downgradeBiosImageOnDut, err := getDowngradeBiosImage(ctx, h.DUT, fwName, tempDirOnDut)
	if err != nil {
		return false, errors.Wrap(err, "failed to get downgrade image")
	}

	if err := compareFmapScheme(ctx, h.DUT, backupOnDut, downgradeBiosImageOnDut); err != nil {
		return false, errors.Wrap(err, "FMap comparison failure")
	}

	originalMeVersion, downgradeMeVersion, isDowngradePossible, err := getCsmeVersions(ctx, h.DUT, tempDirOnDut, backupOnDut, downgradeBiosImageOnDut)
	if err != nil {
		return false, errors.Wrap(err, "failed to get CSME versions")
	}
	if !isDowngradePossible {
		return isDowngradePossible, err
	}

	// Reboot before reading CBMEM log
	s.Log("Reboot and wait for DUT to reconnect")
	if err := h.DUT.Reboot(ctx); err != nil {
		return false, errors.Wrap(err, "failed to reboot DUT")
	}
	activeMeVersion, err := getActiveCsmeRwVersion(ctx, h.DUT)
	if err != nil {
		return false, errors.Wrap(err, "failed to get active ME version")
	} else if activeMeVersion != originalMeVersion {
		return false, errors.Errorf("Incorrect DUT state. Cannot start the test. Expected ME version is %q, got %q", originalMeVersion, activeMeVersion)
	}
	s.Logf("Active CSME RW Version: %s", activeMeVersion)

	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		return false, errors.Wrap(err, "failed to setup futility instance")

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
			return false, errors.Wrapf(err, "failed to flash downgraded firmware image:\nOutput\n %s", string(out))
		}

		if err := switchSlotAndVerifyCsme(ctx, h.DUT, h.Reporter, slot, downgradeMeVersion); err != nil {
			return false, errors.Wrap(err, "failed to switch to downgraded ME")

		}

		s.Log("Upgrading RW section. Updrade ME Version: ", originalMeVersion)
		originalOpts := futility.NewUpdateOptions(backupOnDut).WithMode(futility.UpdateModeRecovery).WithWriteProtection(futility.WriteProtectionEnable)
		if out, err := futilityInstance.Update(ctx, originalOpts); err != nil {
			return false, errors.Wrapf(err, "failed to flash original firmware image:\nOutput\n %s", string(out))
		}

		if err := switchSlotAndVerifyCsme(ctx, h.DUT, h.Reporter, slot, originalMeVersion); err != nil {
			return false, errors.Wrap(err, "failed to switch to original ME")
		}
		recoveryRequired = false
	}
	return true, nil
}
func performForcedCseSync(ctx context.Context, s *testing.State) error {
	h := s.FixtValue().(*fixture.Value).Helper
	/*
	 * Force test CSE firmware update scenario if below conditions are being met:
	 *  - CSE Update not required
	 *  - VB2_GBB_FLAG_FORCE_CSE_SYNC gbb flag is set,
	 *  - CSE FW is in RO
	 */

	// clear event log
	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		return errors.Wrap(err, "failed to clear event log")
	}

	s.Log("Enabling GBB flag for forced CSE sync")
	req := fwpb.GBBFlagsState{Set: []fwpb.GBBFlag{fwpb.GBBFlag_FORCE_CSE_SYNC}}
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &req); err != nil {
		return errors.Wrap(err, "failed to enable gbb for forced cse sync")
	}

	s.Log("Performing EC reboot")
	if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
		return errors.Wrap(err, "failed to reboot EC")
	}
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "device is not responding post EC reboot")
	}

	// Find the cse sync stage from event log
	syncStage, err := h.Reporter.GetCseSyncStage(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to find cse sync info in elog")
	}

	// Check if cse sync performed at expected stage
	payloadCseSync := s.Features("").Hardware.HardwareFeatures.FwConfig.LateCseSync == api.HardwareFeatures_PRESENT
	if payloadCseSync && syncStage == "Early" {
		return errors.Wrap(err, "CSE is configured for late sync, but performed early")
	}
	if !payloadCseSync && syncStage == "Late" {
		return errors.Wrap(err, "CSE is configured for early sync, but performed late")
	}
	s.Log("Forced CSE sync successful")
	return nil
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
