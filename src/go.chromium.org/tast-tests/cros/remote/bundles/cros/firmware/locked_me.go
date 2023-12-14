// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	lockedMELocalDir        = "/usr/local/share/tast" // Path for data files
	lockedMEBiosFilename    = "bios.bin"              // Filename for bios image
	lockedMERandomFilename  = "newdata"               // Filename for random data image
	lockedMEFlashedFilename = "flasheddata"           // Filename for flashed file
	lockedMEBackupFilename  = "backupdata"            // Filename for flashed file
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LockedME,
		Desc: "Test that the ME region is properly locked",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level5"},
		Requirements: []string{"sys-fw-0024-v01", "sys-fw-0025-v01", "sys-fw-0021-v01"},
		SoftwareDeps: []string{"flashrom"},
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func md5sum(ctx context.Context, s *testing.State, filename string) string {
	// Run md5sum on the file.
	h := s.FixtValue().(*fixture.Value).Helper
	var cmd = "md5sum"
	var ar = filename
	out, err := h.DUT.Conn().CommandContext(ctx, cmd, ar).Output(ssh.DumpLogOnError)

	if err != nil {
		s.Fatal("Failed to run md5sum")
	}
	// Get the md5sum from the output.
	md5sum := strings.Split(string(out), " ")

	s.Logf("%s - md5sum = %x", filename, md5sum[0])
	// Return the md5sum as a 32-character hex string.
	return md5sum[0]
}

func hasME(ctx context.Context, s *testing.State, flashromInstance *flashrom.Instance) bool {

	out, err := flashromInstance.NoOp(ctx)

	if err != nil {
		s.Fatal("hasME: Failed to run flashrom -V")
	}

	if strings.Contains(string(out), "FREG0") {
		return true
	}
	return false
}

func checkManufacturingMode(ctx context.Context, s *testing.State) {
	// Fail if manufacturing mode is not found or is enabled.
	// See if coreboot told us that the ME is still in Manufacturing Mode.
	// It shouldn't be. We have to look only at the last thing it reports
	// because it reports the values twice and the first one isn't always
	// reliable.
	h := s.FixtValue().(*fixture.Value).Helper
	s.Log("Check that we're not in Manufacturing Mode")
	firmwareLog, err := h.DUT.Conn().CommandContext(
		ctx, "cat", "/sys/firmware/log").Output(ssh.DumpLogOnError)

	if err != nil {
		s.Fatal("Failed to run cat /sys/firmware/log")
	}

	lines := strings.Split(string(firmwareLog), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "ME: Manufacturing Mode") {
			if strings.Contains(lines[i], "YES") {
				s.Fatal("The ME is still in Manufacturing Mode")
			}
			break
		}
	}
}

func restoreBiosImage(ctx context.Context, s *testing.State, flashromInstance *flashrom.Instance) {
	biosImagePath := path.Join(lockedMELocalDir, lockedMEBiosFilename)
	_, err := flashromInstance.Write(ctx,
		"",
		true,  // --noverify-all
		false, // --noverify
		"",    // -flash-contents
		[]string{biosImagePath})
	if err != nil {
		s.Fatal("restoring failed - flashromInstance.Write Failed: ", err)
	}
}

func checkRegionInaccessible(ctx context.Context, s *testing.State,
	flashromInstance *flashrom.Instance, sectname string) {
	// If we can modify the ME section, restore it and raise an error.
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Try to write section %s", sectname)

	// get file size:
	lsCmd := "ls -ltr " + path.Join(lockedMELocalDir, sectname) + "| cut -d \" \" -f 5"
	fsizeBarr, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", lsCmd).Output(ssh.DumpLogOnError)
	fsizeStr := strings.Replace(string(fsizeBarr), "\n", "", -1)
	if err != nil {
		s.Fatalf("Couldn't find file %s : %s", path.Join(lockedMELocalDir, sectname), err)
	}
	_, err = strconv.Atoi(string(fsizeStr))
	if err != nil {
		s.Fatalf("Could not convert string to int: %s : %s", fsizeStr, err)
	}

	regionBackupPath := path.Join(lockedMELocalDir, lockedMEBackupFilename)
	_, err = flashromInstance.Read(ctx,
		"",
		[]string{fmt.Sprintf("%s:%s", sectname, regionBackupPath)})
	if err != nil {
		s.Fatalf("Failed to read %s: %s", sectname, err)
	}

	// generate a file with random data sized the same as the section we're writing.
	randomImagePath := path.Join(lockedMELocalDir, lockedMERandomFilename)
	args := "dd if=/dev/urandom of=" + randomImagePath + " count=1 bs=" + string(fsizeStr)
	ddRes, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", args).Output(ssh.DumpLogOnError)

	if err != nil {
		s.Fatalf("Failed to create a random file to write: %s, %s", err, ddRes)
	}

	// Noticed that flashrom_library doesn't return an error when Write fails.
	// We check in this test if data was changed in these regions.

	_, _ = flashromInstance.Write(ctx,
		"",
		true,  // --noverify-all
		false, // --noverify
		"",    // -flash-contents
		[]string{fmt.Sprintf("%s:%s", sectname, randomImagePath)})

	flashedPath := path.Join(lockedMELocalDir, lockedMEFlashedFilename)
	_, err = flashromInstance.Read(ctx,
		"",
		[]string{fmt.Sprintf("%s:%s", sectname, flashedPath)})
	if err != nil {
		// in case we were able to write, restore from backup
		restoreBiosImage(ctx, s, flashromInstance)
		s.Fatalf("Failed to read %s: %s", sectname, err)
	}

	md5sumBackup := md5sum(ctx, s, regionBackupPath)
	md5sumFlashed := md5sum(ctx, s, flashedPath)
	if md5sumBackup != md5sumFlashed {
		s.Log("Oops, it worked! Put it back")
		restoreBiosImage(ctx, s, flashromInstance)
		s.Fatalf("MD5s are different - that means we were able to region %s", sectname)
	} else {
		s.Logf("MD5s are equal.. That's good! Region %s is Locked", sectname)
	}
}

// LockedME makes sure ME is locked. It fails otherwise.
func LockedME(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	// Configure flashrom instance
	var flashromConfig flashrom.Config
	flashromInstance, ctx, fullShutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityDebug).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := fullShutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()
	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
	}

	wpStatus, out, err := flashromInstance.SoftwareWriteProtectStatus(ctx)
	if err != nil {
		s.Fatalf("Could not check WP status : %s , %s", err, out)
	}
	if wpStatus {
		s.Log("Software wp is enabled on the AP's SPI-ROM, " +
			"or a protected range is set. Trying to disable it")
		out, err = flashromInstance.SoftwareWriteProtectDisable(ctx)
		if err != nil {
			s.Fatalf("Could not disable WP : %s , %s", err, out)
		}
	}

	//See if the system even has an ME, and whether we expected that.
	if !hasME(ctx, s, flashromInstance) {
		s.Fatal("No ME found. That's probably wrong")
	}

	// Make sure manufacturing mode is off.
	checkManufacturingMode(ctx, s)

	// Read the image using flashrom.
	biosImagePath := path.Join(lockedMELocalDir, lockedMEBiosFilename)
	out, err = flashromInstance.Read(ctx, string(biosImagePath), nil /* entire chip */)

	if err != nil {
		s.Fatalf("Flashrom read failed: %s, %s", err, out)
	}

	// Use 'IFWI' fmap region as a proxy for a device which doesn't
	// have a dedicated ME region in the boot media.
	fmapStr, err := h.DUT.Conn().CommandContext(
		ctx, "dump_fmap", "-p", biosImagePath).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("dump_fmap failed: %s, %s", err, fmapStr)
	}

	var isIfwiPlatform = strings.Contains(string(fmapStr), "IFWI")

	// Get the bios image and extract the ME components
	s.Log("Pull the ME components from the BIOS")

	var inaccessibleSection = ""
	if isIfwiPlatform {
		inaccessibleSection = "DEVICE_EXTENSION"
	} else {
		inaccessibleSection = "SI_ME"
	}

	section1 := fmt.Sprintf("%s:%s", "SI_DESC", path.Join(lockedMELocalDir, "SI_DESC"))
	section2 := fmt.Sprintf("%s:%s", inaccessibleSection, path.Join(lockedMELocalDir, inaccessibleSection))

	_, err = h.DUT.Conn().CommandContext(
		ctx, "dump_fmap", "-x", biosImagePath, section1, section2).Output(ssh.DumpLogOnError)

	if err != nil {
		s.Fatalf("dump_fmap failed: %s", err)
	} else {
		s.Logf("writen %s and %s", path.Join(lockedMELocalDir, "SI_DESC"), path.Join(lockedMELocalDir, inaccessibleSection))
	}

	// So far, so good, but we need to be certain. Rather than parse what
	// flashrom tells us about the ME-related registers, we'll just try to
	// change the ME components. We shouldn't be able to.
	checkRegionInaccessible(ctx, s, flashromInstance, inaccessibleSection)
	checkRegionInaccessible(ctx, s, flashromInstance, "SI_DESC")

	// Okay, that's about all we can try. Looks like it's locked.

}
