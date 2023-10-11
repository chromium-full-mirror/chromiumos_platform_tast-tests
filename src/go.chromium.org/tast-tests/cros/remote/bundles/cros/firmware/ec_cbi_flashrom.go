// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	// 	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

var tmpCbiFlashromDir = filepath.Join("/", "mnt", "stateful_partition", "cbi_flashrom.XXXXXX")

func init() {
	testing.AddTest(&testing.Test{
		Func: ECCbiFlashrom,
		Desc: "Test that flashrom don't write or erase the CBI section on EC flash",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jasonyuan@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:    []string{"group:firmware", "firmware_unstable"},
		Fixture: fixture.NormalMode,
		Timeout: 15 * time.Minute,
	})
}

func ECCbiFlashrom(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	cbiSectionName := "CBI"
	oldCbiImageName := "old_cbi.bin"
	newCbiImageName := "new_cbi.bin"
	cbiImageSize := 256

	s.Log("Disabling write protect")
	if err := setECWriteProtect(ctx, h, false); err != nil {
		s.Fatal("Failed to disable write protect: ", err)
	}

	s.Log("Create temp dir in DUT")
	workPathByteArr, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", tmpCbiFlashromDir).Output(ssh.DumpLogOnError)

	workPath := strings.TrimSpace(string(workPathByteArr))
	if err != nil {
		s.Fatal("Failed to create temp dirs: ", err)
	}
	s.Log("Created temp directory at: ", workPath)
	oldImagePath := filepath.Join(workPath, oldCbiImageName)
	newImagePath := filepath.Join(workPath, newCbiImageName)

	defer func() {
		s.Log("Delete temp dir and contained files from DUT")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", "-r", workPath).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete temp dir: ", err)
		}
	}()

	err = getCbiImg(ctx, h, oldImagePath, cbiImageSize)
	if err != nil {
		s.Fatal("Failed to backup CBI image: ", err)
	}

	defer func() {
		s.Log("Recovering CBI on EC flash")
		if err := setCbiImg(ctx, h, oldImagePath, cbiImageSize); err != nil {
			s.Fatal("Expected to recover CBI: ", err)
		}
	}()

	if err := verifySection(ctx, h, workPath, cbiSectionName); err != nil {
		s.Fatal("Expected Verify to succeed: ", err)
	}

	s.Log("Corrupting CBI section through Flashrom")
	if err := corruptSectionCbi(ctx, h, workPath, cbiSectionName); err != nil {
		s.Fatal("Expected a successful attempt to corrupt CBI section: ", err)
	}

	err = getCbiImg(ctx, h, newImagePath, cbiImageSize)
	if err != nil {
		s.Fatal("Expected read to succeed: ", err)
	}

	diff, err := h.DUT.Conn().CommandContext(ctx, "diff", newImagePath, oldImagePath).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to compare old vs new cbi: ", err)
	}

	if len(diff) != 0 {
		s.Fatalf("New image is not matching old image, got %s: %v", string(diff), err)
	}
}

func getCbiImg(ctx context.Context, h *firmware.Helper, file string, size int) error {
	out, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).CBIBin(ctx, firmware.CBIBinRead, file, strconv.Itoa(size))
	if err != nil {
		return errors.Wrapf(err, "failed to read from cbi, got output: %v", out)
	}

	return nil
}

func setCbiImg(ctx context.Context, h *firmware.Helper, file string, size int) error {
	out, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).CBIBin(ctx, firmware.CBIBinWrite, file, strconv.Itoa(size))
	if err != nil {
		return errors.Wrapf(err, "failed to read from cbi, got output: %v", out)
	}

	return nil
}

func verifySection(ctx context.Context, h *firmware.Helper, workPath, section string) (retErr error) {
	sectionOffset, sectionSize, err := getSectionInfo(ctx, h, workPath, section)
	testing.ContextLog(ctx, "Read following section from Flashrom: ", section)

	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerEc, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to shutdown flashromInstance")
			} else {
				testing.ContextLog(ctx, "Failed to shutdown flashromInstance: ", err)
			}
		}
	}()
	if err != nil {
		errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	// Temp file to hold image section.
	tempSectionPath := filepath.Join(workPath, "img.XXXXXX")
	sectionPathByteArr, err := h.DUT.Conn().CommandContext(ctx, "mktemp", tempSectionPath).Output(ssh.DumpLogOnError)
	sectionPath := strings.TrimSpace(string(sectionPathByteArr))

	// Temp file to hold reference image section.
	tempRefSectionPath := filepath.Join(workPath, "img_ref.XXXXXX")
	refSectionPathByteArr, err := h.DUT.Conn().CommandContext(ctx, "mktemp", tempRefSectionPath).Output(ssh.DumpLogOnError)
	refSectionPath := strings.TrimSpace(string(refSectionPathByteArr))

	// Create a reference mask of the section.
	// The reference mask is first created as file of size sectionOffset+sectionSize, filled with 0xFF.
	if _, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf(`head -c %d < /dev/zero | tr '\0' '\377' > %s`, sectionOffset+sectionSize, refSectionPath)).Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create cbi reference mask")
	}

	// The initial sectionOffset bytes of the reference mask is replaced with 0x00.
	// We end up with a reference mask that has sectionOffset bytes of 0x00 followed by sectionSize bytes of 0xFF
	if _, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf(`dd if=/dev/zero ibs=1 count=%d of=%s conv=notrunc`, sectionOffset, refSectionPath)).Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to adjust cbi reference mask offset")
	}

	if out, err := flashromInstance.Read(ctx, sectionPath, []string{section}); err != nil {
		return errors.Wrapf(err, "failed to read Flashrom: %s", string(out))
	}

	if out, err := h.DUT.Conn().CommandContext(ctx, "cmp", "-n", strconv.Itoa(sectionOffset+sectionSize), sectionPath, refSectionPath).CombinedOutput(ssh.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "expected CBI section not visible to Flashrom: %s", string(out))
	}

	testing.ContextLog(ctx, "Delete temp files at path ", workPath)
	if _, err = h.DUT.Conn().CommandContext(ctx, "rm", sectionPath).Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to delete temp file")
	}
	if _, err = h.DUT.Conn().CommandContext(ctx, "rm", refSectionPath).Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to delete temp file")
	}

	return nil
}

func corruptSectionCbi(ctx context.Context, h *firmware.Helper, workPath, section string) (retErr error) {
	_, sectionSize, err := getSectionInfo(ctx, h, workPath, section)
	if err != nil {
		return errors.Wrap(err, "failed to get section info")
	}
	if sectionSize == 0 {
		return errors.Errorf("didn't find %q section in fmap", section)
	}

	// Temp file to hold corrupted image section.
	tempSectionPath := filepath.Join(workPath, "img.XXXXXX")
	sectionPathByteArr, err := h.DUT.Conn().CommandContext(ctx, "mktemp", tempSectionPath).Output(ssh.DumpLogOnError)
	sectionPath := strings.TrimSpace(string(sectionPathByteArr))

	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerEc, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to shutdown flashromInstance")
			} else {
				testing.ContextLog(ctx, "Failed to shutdown flashromInstance: ", err)
			}
		}
	}()
	if err != nil {
		errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	ddArgs := []string{
		"if=/dev/urandom", fmt.Sprintf("of=%s", sectionPath),
		"bs=1", fmt.Sprintf("count=%d", sectionSize),
	}
	testing.ContextLogf(ctx, "Generate random file of size: %d to path %v", sectionSize, sectionPath)
	if _, err = h.DUT.Conn().CommandContext(ctx, "dd", ddArgs...).Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create random file with dd cmd")
	}

	testing.ContextLog(ctx, "Write random file to section")

	if out, err := flashromInstance.Write(ctx, "", false, true, "", []string{fmt.Sprintf("%s:%s", section, sectionPath)}); err != nil {
		return errors.Wrapf(err, "failed to run flashrom cmd: %s", string(out))
	}

	testing.ContextLog(ctx, "Delete temp file at path ", sectionPath)
	if _, err = h.DUT.Conn().CommandContext(ctx, "rm", sectionPath).Output(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to delete temp file")
	}
	return nil
}

func getSectionInfo(ctx context.Context, h *firmware.Helper, workPath, section string) (sectionOffset, sectionSize int, retErr error) {
	// Temp file to hold current image WP_RO.
	tempSectionPath := filepath.Join(workPath, "img.XXXXXX")
	sectionPathByteArr, err := h.DUT.Conn().CommandContext(ctx, "mktemp", tempSectionPath).Output(ssh.DumpLogOnError)
	sectionPath := strings.TrimSpace(string(sectionPathByteArr))
	testing.ContextLog(ctx, "Read WP_RO section to file ", sectionPath)

	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerEc, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to shutdown flashromInstance")
			} else {
				testing.ContextLog(ctx, "Failed to shutdown flashromInstance: ", err)
			}
		}
	}()
	if err != nil {
		errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	if out, err := flashromInstance.Read(ctx, "", []string{fmt.Sprintf("WP_RO:%s", sectionPath)}); err != nil {
		return 0, 0, errors.Wrapf(err, "failed to run flashrom cmd: %s", string(out))
	}

	testing.ContextLog(ctx, "Checking fmap")
	out, err := h.DUT.Conn().CommandContext(ctx, "dump_fmap", "-p", sectionPath).Output(ssh.DumpLogOnError)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to dump fmap")
	}

	// Format for the dumped fmap is "SectionName offset size".
	sectionMatch := regexp.MustCompile(fmt.Sprintf(`%s\s+(\d+)\s+(\d+)`, section)).FindSubmatch(out)
	if sectionMatch == nil {
		return 0, 0, nil
	}
	sectionOffset, err = strconv.Atoi(string(sectionMatch[1]))
	sectionSize, err = strconv.Atoi(string(sectionMatch[2]))
	testing.ContextLogf(ctx, "Section %q offset: %d size: %d", section, sectionOffset, sectionSize)

	if out, err = h.DUT.Conn().CommandContext(ctx, "rm", sectionPath).Output(ssh.DumpLogOnError); err != nil {
		return 0, 0, errors.Wrap(err, "failed to delete temp file")
	}

	return sectionOffset, sectionSize, nil
}
