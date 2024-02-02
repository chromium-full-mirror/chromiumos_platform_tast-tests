// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"hash"
	"strings"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/testing"
)

var (
	tpmVersion string
)

const (
	keyblockUnknown = 0
	keyblockNormal  = 1
)

type bootModeArgs struct {
	devMode       byte
	recMode       byte
	keyBlockFlags byte
	devsw         string
	mainfw        string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TPMExtend,
		Desc:         "Test to ensure TPM PCRs are extended correctly",
		Contacts:     []string{"digehlot@google.com", "chromeos-firmware@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
				Val: &bootModeArgs{
					devMode:       0,
					recMode:       0,
					keyBlockFlags: keyblockNormal,
					devsw:         "0",
					mainfw:        "normal",
				},
			},
			{
				Name:      "recovery",
				Fixture:   fixture.RecModeNoServices,
				ExtraAttr: []string{"firmware_usb"},
				Val: &bootModeArgs{
					devMode:       0,
					recMode:       1,
					keyBlockFlags: keyblockUnknown,
					devsw:         "0",
					mainfw:        "recovery",
				},
			},
			{
				Name:    "dev",
				Fixture: fixture.DevModeGBB,
				Val: &bootModeArgs{
					devMode:       1,
					recMode:       0,
					keyBlockFlags: keyblockNormal,
					devsw:         "1",
					mainfw:        "developer",
				},
			},
			{
				Name:      "dev_recovery",
				Fixture:   fixture.DevRecModeNoServices,
				ExtraAttr: []string{"firmware_usb"},
				Val: &bootModeArgs{
					devMode:       1,
					recMode:       1,
					keyBlockFlags: keyblockUnknown,
					devsw:         "1",
					mainfw:        "recovery",
				},
			},
		},
	})
}

func tpm1CheckPCR(ctx context.Context, s *testing.State, num int, hashObj hash.Hash) {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Reading PCR%d from the device", num)
	pcrsFile := "/sys/class/*/tpm0/device/pcrs"
	pcrBytes, err := h.DUT.Conn().CommandContext(ctx, "cat", pcrsFile).Output()
	if err != nil {
		s.Fatal("Failed to read TPM1 PCR: ", err)
	}
	var pcr = string(pcrBytes)

	padded := append(make([]byte, 20), hashObj.Sum(nil)[:20]...)
	extended := sha256.Sum256((padded)[:])
	extendedString := fmt.Sprintf("%X", extended)
	spaced := ""
	for i := 0; i < len(extendedString); i += 2 {
		spaced += extendedString[i:i+2] + " "
	}

	extendedString = fmt.Sprintf("PCR-%.2d: %s", num, spaced)

	if !strings.Contains(pcr, extendedString) {
		s.Fatalf("PCR%d was not extended with SHA256 of HWID", num)
	}
}

func tpm2CheckPCR(ctx context.Context, s *testing.State, num int, hashObj hash.Hash) {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Reading PCR%d from the device", num)

	pcrIndex := fmt.Sprintf("--index=%d", num)
	pcrBytes, err := h.DUT.Conn().CommandContext(ctx, "trunks_client", "--read_pcr", pcrIndex).Output()
	if err != nil {
		s.Fatal("Failed to read TPM2 PCR: ", err)
	}
	var pcr = string(pcrBytes)

	padded := append(hashObj.Sum(nil), make([]byte, 12)...)[:32]
	extended := sha256.Sum256((append(make([]byte, 32), padded...))[:])
	extendedString := fmt.Sprintf("%X", extended)

	if !strings.Contains(pcr, extendedString) {
		s.Fatalf("PCR%d was not extended with SHA256 of HWID", num)
	}
}

func checkPCR(ctx context.Context, s *testing.State, num int, hashObj hash.Hash) {
	if strings.Contains(tpmVersion, "1.") {
		tpm1CheckPCR(ctx, s, num, hashObj)
	} else {
		tpm2CheckPCR(ctx, s, num, hashObj)
	}
}

func hwIDCheck(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	// Get the hardware version using 'crossystem hwid'
	s.Log("Verifying HWID digest in PCR1")
	hwVersion, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamHwid)
	if err != nil {
		s.Fatal("Could not determine hardware version: ", err)
	}
	s.Log("HWID reported by device is:", hwVersion)

	hashObj := sha256.New()
	hashObj.Write([]byte(hwVersion))

	checkPCR(ctx, s, 1, hashObj)
}

func bootModeVerify(ctx context.Context, s *testing.State, devsw, mainfw string) {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Verifying bootmode digest in PCR0 in (devsw=%s, mainfw=%s) mode", devsw, mainfw)
	if csMap, err := h.Reporter.Crossystem(ctx, reporters.CrossystemParamDevswBoot,
		reporters.CrossystemParamMainfwType); err != nil {
		s.Fatal("Failed to get crossystem")
	} else if csMap[reporters.CrossystemParamDevswBoot] != devsw ||
		csMap[reporters.CrossystemParamMainfwType] != mainfw {
		s.Fatalf("Expected (devsw, mainfw) to be (%q, %q), got (%q, %q)", devsw, mainfw,
			csMap[reporters.CrossystemParamDevswBoot],
			csMap[reporters.CrossystemParamMainfwType])
	}
}

func TPMExtend(ctx context.Context, s *testing.State) {
	arguments := s.Param().(*bootModeArgs)

	// Get the current TPM version
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	tpmVersion, err := hwsec.NewCmdHelper(cmdRunner).GetTPMVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get TPM version: ", err)
	}
	s.Log("TPM version is:", tpmVersion)

	// Verify hardware id digest
	hwIDCheck(ctx, s)

	// Verify bootmode
	bootModeVerify(ctx, s, arguments.devsw, arguments.mainfw)

	// Verify PCR
	hashObj := sha1.New()
	hashObj.Write([]byte{arguments.devMode, arguments.recMode, arguments.keyBlockFlags})
	s.Log("bootmode:", []byte{arguments.devMode, arguments.recMode, arguments.keyBlockFlags})
	checkPCR(ctx, s, 0, hashObj)
}
