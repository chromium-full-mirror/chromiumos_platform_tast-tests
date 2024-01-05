// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type bpdtHeader struct {
	Sig         uint32
	Cnt         uint16
	Ver         uint8
	Flags       uint8
	Checksum    uint32
	IfwiVersion uint32
	FitVersion  uint64
}

type bpdtEntry struct {
	Type   uint32
	Offset uint32
	Size   uint32
}

type subParDirHeader struct {
	Sig      uint32
	Cnt      uint32
	Ver      uint8
	EntryVer uint8
	Length   uint8
	Reserved uint8
	Name     [4]byte
	Checksum uint32
}

type subParDirEntry struct {
	Name     [12]byte
	Offset   uint32
	Length   uint32
	Reserved uint32
}

type ishManifest struct {
	Reserved      [36]byte
	MajorVersion  uint16
	MinorVersion  uint16
	HotfixVersion uint16
	BuildVersion  uint16
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ISH,
		Desc: "Verify CSE is running the latest ISH firmware provided",
		Contacts: []string{
			"chromeos-faft@google.com",
			"khwon@chromium.org", // Test Author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level5"},
		HardwareDeps: hwdep.D(hwdep.X86(), hwdep.IntelIsh()),
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func ISH(ctx context.Context, s *testing.State) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(flashrom.ProgrammerHost, "").
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()

	if err != nil {
		s.Fatal("Failed flashrom probe, unable to build flashrom instance: ", err)
	}

	// Get ISH version from coreboot log.
	corebootLog, err := testexec.CommandContext(ctx, "cbmem", "-1").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get coreboot log: ", err)
	}
	re := regexp.MustCompile("ISH version: ([0-9.]+)")
	match := re.FindStringSubmatch(string(corebootLog))
	ishCSEVersion := ""
	if len(match) > 1 {
		ishCSEVersion = match[1]
	}

	activeCSESection := "ME_RW_B"

	mainfwAct, err := testexec.CommandContext(ctx, "crossystem", "mainfw_act").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get crossystem mainfw_act: ", err)
	}

	if mainfwAct[0] == 'A' {
		activeCSESection = "ME_RW_A"
	}

	biosFile, err := os.CreateTemp("", "")
	if err != nil {
		s.Fatal("Failed to make tmpfile: ", err)
	}

	defer func() {
		biosFile.Close()
		os.Remove(biosFile.Name())
	}()

	if _, err := flashromInstance.Read(ctx, biosFile.Name(), []string{"FMAP", activeCSESection}); err != nil {
		// Flashrom read operation fails if requested region does not exists.
		// There are devices without ME_RW_(A|B) region (AMD or old Intel platforms) but all of them does not have ISH and do not need the test here,
		// so we check once again to see if ME_RW exists. If it doesn't, we just skip the rest of the test.
		_, err := flashromInstance.Read(ctx, biosFile.Name(), []string{"FMAP"})
		if err == nil {
			cbfsLayout, err := testexec.CommandContext(ctx, "cbfstool", biosFile.Name(), "layout").Output(testexec.DumpLogOnError)
			if err != nil {
				s.Fatal("cbfstool layout failed: ", err)
			}
			if !strings.Contains(string(cbfsLayout), fmt.Sprintf("'%s'", activeCSESection)) {
				// There is no ME_RW region, so we can't test.
				s.Log("ME_RW region is not found")
				return
			}
		}
		s.Fatal("Failed to backup current bios: ", err)
	}

	meBinFile, err := os.CreateTemp("", "")
	if err != nil {
		s.Fatal("Failed to make tmpfile: ", err)
	}
	defer func() {
		meBinFile.Close()
		os.Remove(meBinFile.Name())
	}()

	if err := testexec.CommandContext(ctx, "cbfstool", biosFile.Name(), "extract", "-r", activeCSESection, "-n", "me_rw", "-f", meBinFile.Name()).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("cbfstool extract failed: ", err)
	}

	var header bpdtHeader
	if err := binary.Read(meBinFile, binary.LittleEndian, &header); err != nil {
		s.Fatal("Read Failed: ", err)
	}

	if header.Sig != 0x55aa {
		s.Fatal("Invalid signature in BPDT header: ", header.Sig)
	}

	if header.Ver != 2 {
		s.Log("Test only supports BPDT 1.7")
		return
	}

	for i := 0; i < int(header.Cnt); i++ {
		var entry bpdtEntry
		err = binary.Read(meBinFile, binary.LittleEndian, &entry)
		if err != nil {
			s.Fatal("Read failed: ", err)
		}

		// entry type 8 is ISH.
		if entry.Type == 8 {
			var partHeader subParDirHeader
			var partEntry subParDirEntry

			if _, err := meBinFile.Seek(int64(entry.Offset), 0); err != nil {
				s.Fatal("Seek failed: ", err)
			}

			if err := binary.Read(meBinFile, binary.LittleEndian, &partHeader); err != nil {
				s.Fatal("Read failed: ", err)
			}

			// Ignore invalid partition.
			if partHeader.Sig != 0x44504324 {
				continue
			}

			for j := 0; j < int(partHeader.Cnt); j++ {
				if err := binary.Read(meBinFile, binary.LittleEndian, &partEntry); err != nil {
					s.Fatal("Read failed: ", err)
				}

				if string(bytes.TrimRight(partEntry.Name[:], "\x00")) == "ISHC.man" {
					var manifest ishManifest

					if _, err := meBinFile.Seek(int64(entry.Offset+partEntry.Offset), 0); err != nil {
						s.Fatal("Seek failed: ", err)
					}

					if err := binary.Read(meBinFile, binary.LittleEndian, &manifest); err != nil {
						s.Fatal("Read failed: ", err)
					}
					flashVersion := fmt.Sprintf("%d.%d.%d.%d", manifest.MajorVersion,
						manifest.MinorVersion, manifest.HotfixVersion, manifest.BuildVersion)
					if ishCSEVersion != flashVersion {
						s.Fatalf("ISH version differs: %s from CSE and %s from SPI", ishCSEVersion, flashVersion)
					}
					return
				}
			}
		}
	}

	// ISH is not found from SPI flash, so coreboot should not report it.
	if ishCSEVersion != "" {
		s.Fatal("ISH fw is not in SPI flash but coreboot reported: ", ishCSEVersion)
	}

	s.Log("ISH is not found in SPI and FW log")
}
