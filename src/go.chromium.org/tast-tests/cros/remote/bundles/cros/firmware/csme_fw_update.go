// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

var (
	originalBios        string
	downgradeBios       string
	originalMe          string
	downgradeMe         string
	meImageRelativePath string
	spiMeVersion        string
	downgradeMeVersion  string
	activeMeVersion     string
	homeDir             string
	shellballDir        string
	imageDir            string
	fwUpdaterDir        string
	isDowngradePossible bool
)

const (
	defaultTempPath = "/usr/local/tmp/"
	defaultUpdater  = "/usr/sbin/chromeos-firmwareupdate"
	meBlob          = "me_rw.version"
	fwSectionA      = "FW_MAIN_A"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CsmeFwUpdate,
		Desc:         "Verifies that CSME RW firmware can be upgraded or downgraded using chromeos-firmwareupdate --mode=recovery",
		Contacts:     []string{"digehlot@google.com", "chromeos-firmware@google.com"},
		BugComponent: "b:270200529", // ChromeOS > Platform > System > Firmware > FAFT > Infra
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Timeout:      40 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Val:     fixture.NormalMode,
				Fixture: fixture.NormalMode,
			},
			{
				Name:    "dev",
				Val:     fixture.DevModeGBB,
				Fixture: fixture.DevModeGBB,
			},
		},
	})
}

func isCsmeExist(ctx context.Context, s *testing.State, binPath string) bool {
	s.Logf("Checking if %s file present in image: %s ", meBlob, binPath)
	h := s.FixtValue().(*fixture.Value).Helper

	out, err := h.DUT.Conn().CommandContext(ctx, "cbfstool", binPath, "print", "-r", fwSectionA).Output()
	if err != nil {
		s.Fatal("Failed to execute cbfstool: ", err)
	}
	outs := string(out)
	if !strings.Contains(outs, meBlob) {
		s.Logf("%s is not present", meBlob)
		return false
	}
	return true
}

func getFwName(ctx context.Context, s *testing.State) string {
	h := s.FixtValue().(*fixture.Value).Helper

	// Get the firmware name using 'crossystem fwid'.
	fwName, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		s.Fatal("Could not determine firmware version: ", err)
	}
	re := regexp.MustCompile(`Google_([a-z-A-Z-0-9]*)\.(\d*)\.\d*.\d*`)
	match := re.FindStringSubmatch(fwName)
	if len(match) != 3 {
		s.Fatalf("Unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, fwName)
	}
	fwName = strings.ToLower(match[1])
	s.Log("Firmware Version: ", fwName)
	return fwName
}

func getCurrentBiosImage(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	s.Log("Backup current bios image")
	fwBios, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Programmer: pb.Programmer_BIOSProgrammer,
	})

	if err != nil {
		s.Fatal("Failed to backup firmware section: ", err)
	}

	originalBios = imageDir + "bios_original.bin"
	// Copy Bios to tmp dir
	if err := h.DUT.Conn().CommandContext(ctx, "mv", fwBios.Path, originalBios).Run(); err != nil {
		s.Fatalf("Failed to copy %s to %s: %s", fwBios.Path, originalBios, err)
	}

	s.Log("SPI Bios is stored at: ", originalBios)
	if !isCsmeExist(ctx, s, originalBios) {
		s.Fatal("me_rw.version not present in image, Skipping test")
	}

}

func getDowngradeBiosImage(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	s.Logf("Copying bios image from update shellball to %s for downgrade test", downgradeBios)
	fwName := getFwName(ctx, s)

	// Get relative image path
	chromeosFirmwareUpdateManifest := fmt.Sprintf("chromeos-firmwareupdate --manifest | jq -c .%s.host.image", fwName)
	imagePathBytes, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", chromeosFirmwareUpdateManifest).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("chromeos-firmwareupdate --manifest read failed: ", err)
	}
	meImageRelativePath = string(imagePathBytes)
	meImageRelativePath = strings.Trim(meImageRelativePath, "\" \n")
	// Unpack image to DUT temporary directory
	if err := h.DUT.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "--unpack", shellballDir).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to downgrade bios: ", err)
	}
	shellballBios := shellballDir + meImageRelativePath
	downgradeBios = imageDir + "bios_downgrade.bin"

	// Copy Bios to tmp dir
	if err := h.DUT.Conn().CommandContext(ctx, "mv", shellballBios, downgradeBios).Run(); err != nil {
		s.Fatalf("Failed to copy %s to %s: %s", shellballBios, downgradeBios, err)
	}

	s.Log("Downgrade Bios is stored at: ", downgradeBios)
	if !isCsmeExist(ctx, s, downgradeBios) {
		s.Fatal("me_rw.version not present in image, Skipping test")
	}
}

func compareFmapScheme(ctx context.Context, s *testing.State) {
	var sectionsString []string
	dut, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	futilityInstance, err := futility.NewLocalBuilder(dut.DUT()).Build()
	if err != nil {
		s.Fatal("Failed to get futility instance: ", err)
	}
	fmapOriginalBios, _, err := futilityInstance.DumpFmap(ctx, originalBios, append(sectionsString, "ME_RW_A"))
	if err != nil {
		s.Fatal("Failed to run futility dump_fmap: ", err)
	}
	fmapDowngradeBios, _, err := futilityInstance.DumpFmap(ctx, downgradeBios, append(sectionsString, "ME_RW_A"))
	if err != nil {
		s.Fatal("Failed to run futility dump_fmap: ", err)
	}
	s.Log("fmap SPI Image Bios: ", fmapOriginalBios)
	s.Log("fmap Downgrade Bios: ", fmapDowngradeBios)

	if (len(fmapOriginalBios) == 0) != (len(fmapDowngradeBios) == 0) {
		s.Fatal("Test setup issue : FMAP format is different in current and downgrade bios")
	}
}

func cbfsRead(ctx context.Context, s *testing.State, binPath, region, blob, filename string) {
	h := s.FixtValue().(*fixture.Value).Helper

	extractCmd := fmt.Sprintf("cbfstool %s extract -r %s -n %s -f %s", binPath, region, blob, filename)
	out, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", extractCmd).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("cbfstool failed to extract binary: %s, %v", string(out), err)
	}
}

func getImageCsmeRwVersion(ctx context.Context, s *testing.State, binPath, filename string) string {
	h := s.FixtValue().(*fixture.Value).Helper

	cbfsRead(ctx, s, binPath, fwSectionA, meBlob, filename)

	meVersionExtractCtx := fmt.Sprintf("hexdump -C %s |  cut -c 9- | cut -d'|' -f 2", filename)
	meVersionBytes, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", meVersionExtractCtx).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to parse CSME version from ME binary: ", err)
	}

	var meVersion = string(meVersionBytes)
	meVersion = strings.Trim(meVersion, ". \n")
	return meVersion
}

func getActiveCsmeRwVersion(ctx context.Context, s *testing.State) string {
	h := s.FixtValue().(*fixture.Value).Helper

	// Get CSE version from coreboot log.
	const cbmemCommand = "cbmem -c | grep cse_lite:"
	corebootLog, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", cbmemCommand).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get coreboot log: ", err)
	}

	// Parse CSME string in coreboot log.
	re := regexp.MustCompile(`cse_lite: RW version = ([0-9\.]+)`)
	match := re.FindStringSubmatch(string(corebootLog))
	csmeVersion := ""
	if len(match) > 1 {
		csmeVersion = match[1]
	}
	return csmeVersion
}

func cmpLocalFiles(ctx context.Context, s *testing.State, file1, file2 string) string {
	h := s.FixtValue().(*fixture.Value).Helper
	out, err := h.DUT.Conn().CommandContext(ctx, "cmp", file2, file2).Output()
	if err != nil {
		s.Fatal("compare command failed: ", err)
	}
	return string(out)
}

func isMeRwBlobsIdentical(ctx context.Context, s *testing.State) bool {
	downgradeRw := imageDir + "rw_downgrade.bin"
	spiRwA := imageDir + "rw_spi_a.bin"
	spiRwB := imageDir + "rw_spi_b.bin"
	cbfsRead(ctx, s, downgradeBios, "ME_RW_A", "me_rw", downgradeRw)
	cbfsRead(ctx, s, originalBios, "ME_RW_A", "me_rw", spiRwA)
	cbfsRead(ctx, s, originalBios, "ME_RW_B", "me_rw", spiRwB)

	s.Log("Comparing ME blobs")
	diffA := cmpLocalFiles(ctx, s, downgradeRw, spiRwA)
	diffB := cmpLocalFiles(ctx, s, downgradeRw, spiRwB)

	if diffA != "" && diffB != "" {
		s.Log("CSME RW version is same, but downgrade me_rw differs from both me_rw blobs in spi flash")
	} else if diffA != "" {
		s.Log("CSME RW version is same, but downgrade me_rw and FW_MAIN_A me_rw differ")
	} else if diffB != "" {
		s.Log("CSME RW version is same, but downgrade me_rw and FW_MAIN_B me_rw differ")
	} else {
		return true
	}
	return false
}

func getCsmeVersions(ctx context.Context, s *testing.State) {
	// Get the version of me_rw in the spi bios
	originalMe = imageDir + "me_original.bin"
	spiMeVersion = getImageCsmeRwVersion(ctx, s, originalBios, originalMe)

	// Get the version of me_rw in the downgrade bios
	downgradeMe = imageDir + "me_downgrade.bin"
	downgradeMeVersion = getImageCsmeRwVersion(ctx, s, downgradeBios, downgradeMe)

	// Get active CSME RW version from cbmem -1
	activeMeVersion = getActiveCsmeRwVersion(ctx, s)
	s.Logf("Active CSME RW Version                 : %s", activeMeVersion)
	s.Logf("FW main CSME RW Version SPI Image      : %s", spiMeVersion)
	s.Logf("FW main CSME RW Version downgrade Image: %s", downgradeMeVersion)

	isDowngradePossible = true
	if spiMeVersion == downgradeMeVersion {
		isDowngradePossible = !isMeRwBlobsIdentical(ctx, s)
	}
}
func runShellball(ctx context.Context, s *testing.State, binPath, affix string) {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Preparing %s shellball with %s", affix, binPath)

	firmwareUpdater := fwUpdaterDir + "chromos-firmwareupdate-" + affix

	// Copy chromos-firmwareupdate to temporary directory
	if err := h.DUT.Conn().CommandContext(ctx, "cp", defaultUpdater, firmwareUpdater).Run(); err != nil {
		s.Fatalf("Failed to copy %s to %s: %s", defaultUpdater, originalBios, err)
	}

	// unpack bios images
	biosImages := fwUpdaterDir + "bios_images_" + affix + "/"
	if err := h.DUT.Conn().CommandContext(ctx, "sh", firmwareUpdater, "--unpack", biosImages).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to extract bios images: ", err)
	}

	// update bios image
	if err := h.DUT.Conn().CommandContext(ctx, "cp", binPath, biosImages+meImageRelativePath).Run(); err != nil {
		s.Fatalf("Failed to copy %s to %s: %s", binPath, biosImages+meImageRelativePath, err)
	}

	// repack bios image
	if err := h.DUT.Conn().CommandContext(ctx, "sh", firmwareUpdater, "--repack", biosImages).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to repack bios images: ", err)
	}

	// run shell ball
	if err := h.DUT.Conn().CommandContext(ctx, "sh", firmwareUpdater, "--mode=recovery", "--host_only", "--wp=1").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to downgrade bios: ", err)
	}
}

func initDirectory(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	tempdir, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", defaultTempPath+"CSME_XXXXXXXX").Output()
	if err != nil {
		s.Fatal("Failed to create remote data path directory: ", err)
	}
	homeDir = strings.TrimSpace(string(tempdir)) + "/"
	s.Log("Test home directory: ", homeDir)

	shellballDir = homeDir + "bios_images/"
	imageDir = homeDir + "generated_images/"
	fwUpdaterDir = homeDir + "fw_updater/"

	if _, err := h.DUT.Conn().CommandContext(ctx, "mkdir", "-p", fwUpdaterDir).Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to create temp dir: ", err)
	}
	if _, err := h.DUT.Conn().CommandContext(ctx, "mkdir", "-p", imageDir).Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to create image path dir: ", err)
	}
}

func switchSlotAndVerifyCsme(ctx context.Context, s *testing.State, slot, operation, expectedMe string) {
	s.Logf("Switching to slot %s", slot)
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", fmt.Sprintf("fw_try_next=%s", slot)).Run(); err != nil {
		s.Fatal("Failed to set crossystem fw_try_next: ", err)
	}

	s.Log("Reboot and wait for DUT to reconnect")
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// Get active CSME RW version from cbmem -1
	activeMeVersion = getActiveCsmeRwVersion(ctx, s)
	s.Logf("Active CSME RW Version after %s: %s", operation, activeMeVersion)

	if activeMeVersion != expectedMe {
		s.Fatalf("CSME RW %s using FW_MAIN_%s is Failed", operation, slot)
	}
	s.Logf("Slot %s: %s successful", slot, operation)
}

// CsmeFwUpdate tests csme rw firmware update feature by changing the me_rw
// image in firmware main regions with a different version
func CsmeFwUpdate(ctx context.Context, s *testing.State) {
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()
	h := s.FixtValue().(*fixture.Value).Helper
	initDirectory(ctx, s)
	defer func(ctx context.Context) {
		s.Log("Delete temporary test home directory and contained files from DUT")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", "-rf", homeDir).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete test home directory: ", err)
		}
	}(cleanupContext)

	getCurrentBiosImage(ctx, s)
	getDowngradeBiosImage(ctx, s)
	compareFmapScheme(ctx, s)
	getCsmeVersions(ctx, s)
	if !isDowngradePossible {
		s.Fatal("CSME RW blobs are same in downgrade and spi bios")
		return
	}

	for _, slot := range []string{"A", "B"} {
		operation := "downgrade"
		s.Log("Downgrading RW section. Downgrade ME Version : ", downgradeMeVersion)
		runShellball(ctx, s, downgradeBios, operation)

		// Switch Slot and reboot
		switchSlotAndVerifyCsme(ctx, s, slot, operation, downgradeMeVersion)

		operation = "upgrade"
		s.Log("Upgrading RW section. Updrade ME Version : ", spiMeVersion)
		runShellball(ctx, s, originalBios, operation)

		// Switch Slot and reboot
		switchSlotAndVerifyCsme(ctx, s, slot, operation, spiMeVersion)
	}
}
