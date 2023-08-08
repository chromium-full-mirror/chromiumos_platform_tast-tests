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
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TPMExtend,
		Desc:         "Test to ensure TPM PCRs are extended correctly.",
		Contacts:     []string{"digehlot@google.com", "chromeos-firmware@google.com"},
		BugComponent: "b:194910917", // ChromeOS > Platform > System > Firmware > FAFT > Infra
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Params: []testing.Param{
			{
				Name:    "normal",
				Val:     fixture.NormalMode,
				Fixture: fixture.NormalMode,
			},
			{
				Name:      "recovery",
				Val:       fixture.RecModeNoServices,
				Fixture:   fixture.RecModeNoServices,
				ExtraAttr: []string{"firmware_usb"},
			},
			{
				Name:    "dev",
				Val:     fixture.DevModeGBB,
				Fixture: fixture.DevModeGBB,
			},
			{
				Name:      "dev_recovery",
				Val:       fixture.DevRecModeNoServices,
				Fixture:   fixture.DevRecModeNoServices,
				ExtraAttr: []string{"firmware_usb"},
			},
		},
	})
}

func tpm1CheckPCR(ctx context.Context, s *testing.State, num string, hashObj hash.Hash) bool {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Reading PCR%s from the device", num)
	pcrs_file := "/sys/class/*/tpm0/device/pcrs"
	pcr_bytes, _ := h.DUT.Conn().CommandContext(ctx, "cat", pcrs_file).Output()
	var pcr = string(pcr_bytes)

	padded := append(make([]byte, 20), hashObj.Sum(nil)[:20]...)
	extended := sha256.Sum256((padded)[:])
	extended_string := fmt.Sprintf("%X", extended)
	spaced := ""
	for i := 0; i < len(extended_string); i += 2 {
		spaced += extended_string[i:i+2] + " "
	}
	num_int, _ := strconv.Atoi(num)
	extended_string = fmt.Sprintf("PCR-%.2d: %s", num_int, spaced)

	return strings.Contains(pcr, extended_string)
}

func tpm2CheckPCR(ctx context.Context, s *testing.State, num string, hashObj hash.Hash) bool {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Reading PCR%s from the device", num)
	pcr_bytes, _ := h.DUT.Conn().CommandContext(ctx, "trunks_client", "--read_pcr", "--index="+num).Output()
	var pcr = string(pcr_bytes)

	padded := append(hashObj.Sum(nil), make([]byte, 12)...)[:32]
	extended := sha256.Sum256((append(make([]byte, 32), padded...))[:])
	extended_string := fmt.Sprintf("%X", extended)

	return strings.Contains(pcr, extended_string)
}

func checkPCR(ctx context.Context, s *testing.State, num string, hashObj hash.Hash) bool {

	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	tpmVersion, err := hwsec.NewCmdHelper(cmdRunner).GetTPMVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get TPM version ", err)
	}
	s.Log("TPM version is:", tpmVersion)

	if strings.Contains(tpmVersion, "1.") {
		return tpm1CheckPCR(ctx, s, num, hashObj)
	} else {
		return tpm2CheckPCR(ctx, s, num, hashObj)
	}
}

func hwIdCheck(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	// Get the firmware version using 'crossystem fwid'.
	fwVersion, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		s.Fatal("Could not determine firmware version: ", err)
	}
	re := regexp.MustCompile(`Google_([a-z-A-Z-0-9]*)\.(\d*)\.\d*.\d*`)
	match := re.FindStringSubmatch(fwVersion)
	if len(match) != 3 {
		s.Fatalf("Unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, fwVersion)
	}
	fwVersion = match[2]
	s.Log("Firmware Version: ", fwVersion)

	// Get the hardware version using 'crossystem hwid'
	s.Log("Verifying HWID digest in PCR1")
	hwVersion, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamHwid)
	if err != nil {
		s.Fatal("Could not determine hardware version: ", err)
	}
	s.Log("HWID reported by device is:", hwVersion)

	hashObj := sha256.New()
	hashObj.Write([]byte(hwVersion))

	var isExtended = checkPCR(ctx, s, "1", hashObj)
	if !isExtended {
		s.Fatal("PCR1 was not extended with SHA256 of HWID!")
	}
}

func checkPCRBootmode(ctx context.Context, s *testing.State, dev_mode rune, rec_mode rune, keyblock_flags rune) {
	var bootmode = []byte(string(dev_mode) + string(rec_mode) + string(keyblock_flags))
	hashObj := sha1.New()
	hashObj.Write(bootmode)

	var isExtended = checkPCR(ctx, s, "0", hashObj)
	if !isExtended {
		s.Fatal("PCR0 was not extended with SHA256 of HWID!")
	}
}

func bootModeVerify(ctx context.Context, s *testing.State, devsw string, mainfw string) {
	h := s.FixtValue().(*fixture.Value).Helper
	s.Logf("Verifying bootmode digest in PCR0 in (devsw=%s, mainfw=%s) mode", devsw, mainfw)
	if csMap, err := h.Reporter.Crossystem(ctx, reporters.CrossystemParamDevswBoot, reporters.CrossystemParamMainfwType); err != nil {
		s.Fatal("Failed to get crossystem")
	} else if csMap[reporters.CrossystemParamDevswBoot] != devsw || csMap[reporters.CrossystemParamMainfwType] != mainfw {
		s.Fatalf("Expected (devsw, mainfw) to be (%q, %q), got (%q, %q)", devsw, mainfw, csMap[reporters.CrossystemParamDevswBoot], csMap[reporters.CrossystemParamMainfwType])
	}
}

func TPMExtend(ctx context.Context, s *testing.State) {
	s.Log("TPMExtend Test Starts")
	hwIdCheck(ctx, s)
	switch s.Param().(string) {
	case fixture.NormalMode:
		bootModeVerify(ctx, s, "0", "normal")
		// dev_mode: 0, rec_mode: 0, keyblock_flags: "normal" (1)
		checkPCRBootmode(ctx, s, 0, 0, 1)
	case fixture.RecModeNoServices:
		bootModeVerify(ctx, s, "0", "recovery")
		// dev_mode: 0, rec_mode: 1, keyblock_flags: "unknown" (0)
		checkPCRBootmode(ctx, s, 0, 1, 0)
	case fixture.DevModeGBB:
		bootModeVerify(ctx, s, "1", "developer")
		// dev_mode: 1, rec_mode: 0, keyblock_flags: "normal" (1)
		checkPCRBootmode(ctx, s, 1, 0, 1)
	case fixture.DevRecModeNoServices:
		bootModeVerify(ctx, s, "1", "recovery")
		// dev_mode: 1, rec_mode: 1, keyblock_flags: "unknown" (0)
		checkPCRBootmode(ctx, s, 1, 1, 0)
	}
	s.Log("TPMExtend Test Completes")
}
