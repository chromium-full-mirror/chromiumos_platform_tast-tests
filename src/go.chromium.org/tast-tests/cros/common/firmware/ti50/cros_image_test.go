// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"reflect"
	"testing"
)

func TestVersionCommandTi50Processor1(t *testing.T) {
	input := `
RO_A:    0.0.22/ffc7a523
RO_B:  * 0.0.46/c57c2460
RW_A:  * 0.24.61/ti50_common_tot:v0.0.909-efe9eb23
RW_B:    Empty
BID A:   46464646:00000000:00000010 Yes
Build:   ti50_common_tot:v0.0.909-efe9eb23
		 libtock-rs:v0.0.920-5601187
		 tock:v0.0.9651-e191d9f8e
		 ms-tpm-20-ref:v0.0.314-e2ba34f
		 user@host02 2023-11-03 16:41:46
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.22", ImageCheck: "ffc7a523"},
		RoB:   RoInfo{Active: true, Version: "0.0.46", ImageCheck: "c57c2460"},
		RwA:   RwInfo{Empty: false, Active: true, Version: "0.24.61", Branch: ToT},
		RwB:   RwInfo{Empty: true},
		Bid:   BidInfo{Empty: false, BidType: 0x46464646, Mask: 0, Flags: 0x10},
		Build: BuildInfo{Branch: ToT},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandTi50Processor2(t *testing.T) {
	input := `
[       0.823] RO_A:    0.0.52/d1072955
[       0.824] RO_B:  * 0.0.56/cf178281
[       0.827] RW_A:  * 0.24.20/ti50_common:v0.0.2888-c2eeb17e
[       0.833] RW_B:    1.24.30/DBG/ti50_common_tot:v0.0.360+ef9becfc
[       0.839] BID A:   00000000:00000000:00000000 Yes
[       0.845] BID B:   00000000:00000000:00000000 Yes
[       0.850] Build:   ti50_common:v0.0.2888-c2eeb17e
[       0.854]   libtock-rs:v0.0.913-d8d9fdc
[       0.859]   tock:v0.0.9623-bf7357e8b
[       0.863]   ms-tpm-20-ref:v0.0.301-7a188ef
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.52", ImageCheck: "d1072955"},
		RoB:   RoInfo{Active: true, Version: "0.0.56", ImageCheck: "cf178281"},
		RwA:   RwInfo{Empty: false, Active: true, Version: "0.24.20", Branch: Unknown},
		RwB:   RwInfo{Empty: false, Active: false, Debug: true, Version: "1.24.30", Branch: ToT},
		Bid:   BidInfo{Empty: false, BidType: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: Unknown, Debug: false},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandTi50Processor3(t *testing.T) {
	input := `
RO_A:    0.0.52/d1072955
RO_B:  * 0.0.56/cf178281
RW_A:    0.24.20/ti50_common:v0.0.2888-c2eeb17e
RW_B:  * 1.24.30/DBG/ti50_common_tot:v0.0.360+ef9becfc
BID A:   00000000:00000000:00000000 Yes
BID B:   00000000:00000000:00000000 Yes
Build:   DBG/ti50_common_tot:v0.0.360+ef9becfc
         libtock-rs:v0.0.915-10cf494
         tock:v0.0.9632-b3a7d9d57
         ms-tpm-20-ref:v0.0.306-ba9d73d
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.52", ImageCheck: "d1072955"},
		RoB:   RoInfo{Active: true, Version: "0.0.56", ImageCheck: "cf178281"},
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.24.20", Branch: Unknown},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.24.30", Branch: ToT},
		Bid:   BidInfo{Empty: false, BidType: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: ToT, Debug: true},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandCr50Processor1(t *testing.T) {
	input := `
Chip:    g cr50 B2-C
Board:   0
RO_A:  * 0.0.11/bc74f7dc
RO_B:    0.0.11/4d655eab
RW_A:  * 0.3.22/cr50_v1.9308_26_0.596-e6b91d6
RW_B:    0.3.22/cr50_v1.9308_26_0.596-e6b91d6
BID A:   00000000:00000000:00000000 Yes
BID B:   00000000:00000000:00000000 Yes
Build:   0.3.22/cr50_v1.9308_26_0.596-e6b91d6
		 tpm2:v1.9308_26_0.36-6e23777
		 cryptoc:v1.9308_26_0.2-f15cf55
		 2019-10-30 18:21:09 @chromeos-ci-legacy-us-central1-
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: true, Version: "0.0.11", ImageCheck: "bc74f7dc"},
		RoB:   RoInfo{Active: false, Version: "0.0.11", ImageCheck: "4d655eab"},
		RwA:   RwInfo{Empty: false, Active: true, Version: "0.3.22", Branch: Unknown},
		RwB:   RwInfo{Empty: false, Active: false, Version: "0.3.22", Branch: Unknown},
		Bid:   BidInfo{Empty: false, BidType: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: Unknown},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandCr50Processor2(t *testing.T) {
	input := `
Chip:    g cr50 B2-C
Board:   0
RO_A:    0.0.11/bc74f7dc
RO_B:  * 0.0.12/9eb618de
RW_A:    0.6.211/cr50_v3.94_pp.256-f6119fcacf
RW_B:  * 1.6.205/DBG/cr50_v2.0.3597-2b7751b89f
BID A:   46464646:00000000:00000010 Yes
BID B:   00000000:00000000:00000000 Yes
Build:   1.6.205/DBG/cr50_v2.0.3597-2b7751b89f
         tpm2:v1.9308_26_0.80-df48334
         pinweaver:v0.0.143-e90fe74
         2023-06-01 20:42:33 mruthven@mruthven
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.11", ImageCheck: "bc74f7dc"},
		RoB:   RoInfo{Active: true, Version: "0.0.12", ImageCheck: "9eb618de"},
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.6.211", Branch: PrePvt},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.6.205", Branch: ToT},
		Bid:   BidInfo{Empty: false, BidType: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: ToT, Debug: true},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandCr50Processor3(t *testing.T) {
	input := `
Chip:    g cr50 B2-D
Board:   0
RO_A:    0.0.11/bc74f7dc
RO_B:  * 0.0.12/9eb618de
RW_A:    0.5.215/cr50_v4.11_mp.258-bcc62291cf
RW_B:  * 0.6.230/cr50_v3.94_pp.259-e20fb86af1
BID A:   46464646:00000000:00000010 Yes
BID B:   46464646:00000000:00000010 Yes
Build:   0.6.230/cr50_v3.94_pp.259-e20fb86af1
         tpm2:v1.9308_26_0.90-28e6aec
         pinweaver:v0.0.155-fe9e86a
         2024-02-09 04:22:10 @chromeos-ci-firmware-us-central1-b-
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.11", ImageCheck: "bc74f7dc"},
		RoB:   RoInfo{Active: true, Version: "0.0.12", ImageCheck: "9eb618de"},
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.5.215", Branch: MP},
		RwB:   RwInfo{Empty: false, Active: true, Version: "0.6.230", Branch: PrePvt},
		Bid:   BidInfo{Empty: false, BidType: 0x46464646, Mask: 0, Flags: 0x10},
		Build: BuildInfo{Branch: PrePvt, Debug: false},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandCr50Processor4(t *testing.T) {
	input := `
Chip:    g cr50 B2-D
Board:   0
RO_A:    0.0.11/bc74f7dc
RO_B:  * 0.0.12/9eb618de
RW_A:    Error
RW_B:  * 0.6.230/cr50_v3.94_pp.259-e20fb86af1
BID A:   46464646:00000000:00000010 Yes
BID B:   46464646:00000000:00000010 Yes
Build:   0.6.230/cr50_v3.94_pp.259-e20fb86af1
         tpm2:v1.9308_26_0.90-28e6aec
         pinweaver:v0.0.155-fe9e86a
         2024-02-09 04:22:10 @chromeos-ci-firmware-us-central1-b-
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.11", ImageCheck: "bc74f7dc"},
		RoB:   RoInfo{Active: true, Version: "0.0.12", ImageCheck: "9eb618de"},
		RwA:   RwInfo{Empty: true},
		RwB:   RwInfo{Empty: false, Active: true, Version: "0.6.230", Branch: PrePvt},
		Bid:   BidInfo{Empty: false, BidType: 0x46464646, Mask: 0, Flags: 0x10},
		Build: BuildInfo{Branch: PrePvt, Debug: false},
	}

	testVersionInfoMatcher(t, input, expected)
}

func testVersionInfoMatcher(t *testing.T, input string, expected VersionCommandInfo) {
	out, err := matchVersionInfo(input)
	if err != nil {
		t.Fatal("error processing version info:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestRunningSameRW(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: true, Version: "0.3.22", Branch: Unknown},
		RwB: RwInfo{Empty: false, Active: false, Version: "0.3.22", Branch: Unknown},
	}

	// Verify checking the active slot works
	testValidateVersionInfo(t, input, "0.3.22", false, false, true)
	// Verify checking both slots works
	testValidateVersionInfo(t, input, "0.3.22", false, true, true)
}

func TestRunningDifferentRW(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: true, Version: "0.6.211", Branch: PrePvt},
		RwB: RwInfo{Empty: false, Active: false, Version: "1.6.205", Branch: ToT},
	}
	testValidateVersionInfo(t, input, "0.6.211", false, false, true)
	// Verify the isDebug mismatch doesn't match
	testValidateVersionInfo(t, input, "0.6.211", true, false, false)
	// Verify checking both slots doesn't match
	testValidateVersionInfo(t, input, "0.6.211", false, true, false)
	// Verify it doesn't match RW_B
	testValidateVersionInfo(t, input, "1.6.205", false, false, false)
}

func TestRunningDebugRW(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: false, Version: "0.6.211", Branch: PrePvt},
		RwB: RwInfo{Empty: false, Active: true, Debug: true, Version: "1.6.205", Branch: ToT},
	}
	testValidateVersionInfo(t, input, "1.6.205", true, false, true)
	// Verify the isDebug doesn't match
	testValidateVersionInfo(t, input, "1.6.205", false, false, false)
	// Verify checking both slots doesn't match
	testValidateVersionInfo(t, input, "1.6.205", true, true, false)
	// Verify it doesn't match RW_A
	testValidateVersionInfo(t, input, "0.6.211", false, false, false)
}

func testValidateVersionInfo(t *testing.T, input VersionCommandInfo, expectedVersion string, isDebug, checkBothSlots, expectMatch bool) {
	matches := ValidateVersionInfo(input, expectedVersion, isDebug, checkBothSlots)
	if expectMatch && !matches {
		t.Fatalf("unexpected mismatch: %+v did not match %s isDebug(%t) bothSlots(%t)", input, expectedVersion, isDebug, checkBothSlots)
	}
	if !expectMatch && matches {
		t.Fatalf("unexpected match: %+v matched %s isDebug(%t) bothSlots(%t)", input, expectedVersion, isDebug, checkBothSlots)
	}
}

func TestRoInfoMatcherInvalidInput(t *testing.T) {
	if _, err := matchRoInfo(`RO_A:    0.0.22/ffc7a523`, SlotB); err == nil {
		t.Fatal("expected error")
	}

	if _, err := matchRoInfo(`RO_B:    0.0.22/ffc7a523`, SlotA); err == nil {
		t.Fatal("expected error")
	}

	if _, err := matchRoInfo(`RO_A:  ** 0.0.22/ffc7a523`, SlotA); err == nil {
		t.Fatal("expected error")
	}
}

func TestRoInfoMatcher1(t *testing.T) {
	input := `RO_A:    0.0.22/ffc7a523`
	expected := RoInfo{Active: false, Version: "0.0.22", ImageCheck: "ffc7a523"}
	testRoInfoMatcher(t, input, SlotA, expected)
}

func TestRoInfoMatcher2(t *testing.T) {
	input := `RO_B:  * 0.0.46/c57c2460`
	expected := RoInfo{Active: true, Version: "0.0.46", ImageCheck: "c57c2460"}
	testRoInfoMatcher(t, input, SlotB, expected)
}

func testRoInfoMatcher(t *testing.T, input string, slot GscSlot, expected RoInfo) {
	out, err := matchRoInfo(input, slot)
	if err != nil {
		t.Fatal("error processing ro info:", err)
	}
	if out != expected {
		t.Fatal("output mismatch")
	}
}

func TestRwInfoMatcherInvalidInput(t *testing.T) {
	if _, err := matchRwInfo(`RW_B:    Empty`, SlotA); err == nil {
		t.Fatal("expected error")
	}
}

func TestRwInfoMatcher1(t *testing.T) {
	input := `RW_A:  * 0.24.61/ti50_common_tot:v0.0.909-efe9eb23`
	expected := RwInfo{Active: true, Empty: false, Version: "0.24.61", Branch: ToT}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher2(t *testing.T) {
	input := `RW_B:    Empty`
	expected := RwInfo{Empty: true}
	testRwInfoMatcher(t, input, SlotB, expected)
}

func TestRwInfoMatcher3(t *testing.T) {
	input := `RW_A:  * 0.24.60/ti50_common_prepvt-15086.B:v0.0.782-aca516e7`
	expected := RwInfo{Active: true, Empty: false, Version: "0.24.60", Branch: PrePvt}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher4(t *testing.T) {
	input := `RW_A:  * 0.23.60/ti50_common_mp-15224.B:v0.0.729-2ab3d1fb`
	expected := RwInfo{Active: true, Empty: false, Version: "0.23.60", Branch: MP}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher5(t *testing.T) {
	input := `RW_A:  * 0.3.22/cr50_v1.9308_26_0.596-e6b91d6`
	expected := RwInfo{Active: true, Empty: false, Version: "0.3.22", Branch: Unknown}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher6(t *testing.T) {
	input := `RW_B:    Error`
	expected := RwInfo{Empty: true}
	testRwInfoMatcher(t, input, SlotB, expected)
}

func testRwInfoMatcher(t *testing.T, input string, slot GscSlot, expected RwInfo) {
	out, err := matchRwInfo(input, slot)
	if err != nil {
		t.Fatal("error processing rw info:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestBidInfoMatcher1(t *testing.T) {
	input := `BID A:   46464646:00000000:00000010 Yes`
	expected := BidInfo{BidType: 0x46464646, Mask: 0, Flags: 0x10}
	testBidInfoMatcher(t, input, SlotA, expected)
}

func TestBidInfoMatcher2(t *testing.T) {
	input := `BID B:   46464646:00000000:00000010 Yes`
	expected := BidInfo{BidType: 0x46464646, Mask: 0, Flags: 0x10}
	testBidInfoMatcher(t, input, SlotB, expected)
}

func testBidInfoMatcher(t *testing.T, input string, slot GscSlot, expected BidInfo) {
	out, err := matchBidInfo(input, slot)
	if err != nil {
		t.Fatal("error processing bid info:", err)
	}
	if out != expected {
		t.Fatal("output mismatch")
	}
}

func TestBuildInfoMatcher1(t *testing.T) {
	input := `Build:   ti50_common_tot:v0.0.909-efe9eb23`
	expected := BuildInfo{Branch: ToT}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher2(t *testing.T) {
	input := `Build:   ti50_common_prepvt-15086.B:v0.0.782-aca516e7`
	expected := BuildInfo{Branch: PrePvt}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher3(t *testing.T) {
	input := `Build:   ti50_common_mp-15224.B:v0.0.729-2ab3d1fb`
	expected := BuildInfo{Branch: MP}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher4(t *testing.T) {
	input := `Build:   0.3.22/cr50_v1.9308_26_0.596-e6b91d6`
	expected := BuildInfo{Branch: Unknown}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher5(t *testing.T) {
	input := `Build:   0.5.12/cr50_v4.11_28_efi.0-8441619945`
	expected := BuildInfo{Branch: EFI}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher6(t *testing.T) {
	input := `Build:   0.6.241/cr50_v4.08_pp.61-a50884addc`
	expected := BuildInfo{Branch: PrePvt}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher7(t *testing.T) {
	input := `Build:   ti50_common:v0.0.2888-c2eeb17e`
	expected := BuildInfo{Branch: Unknown}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher8(t *testing.T) {
	input := `Build:   DBG/ti50_common_tot:v0.0.360+ef9becfc`
	expected := BuildInfo{Branch: ToT, Debug: true}
	testBuildInfoMatcher(t, input, expected)
}

func testBuildInfoMatcher(t *testing.T, input string, expected BuildInfo) {
	out, err := matchBuildInfo(input)
	if err != nil {
		t.Fatal("error processing bid info:", err)
	}
	if out != expected {
		t.Fatal("output mismatch")
	}
}

func TestGetCCDCapabilities1(t *testing.T) {
	input := `
ccd
State: Locked
Password: none
Flags: 0x00400005
Capabilities: 0x000005555555d555
  UartGscRxAPTx    Y 1=Always
  UartGscTxAPRx    Y 1=Always
  UartGscRxECTx    Y 1=Always
  UartGscTxECRx    Y 1=Always
  UartGscRxFpmcuTx Y 1=Always
  UartGscTxFpmcuRx Y 1=Always
  FlashAP          Y 1=Always
  FlashEC          - 3=IfOpened
  OverrideWP       Y 1=Always
  RebootECAP       Y 1=Always
  GscFullConsole   Y 1=Always
  UnlockNoReboot   Y 1=Always
  UnlockNoShortPP  Y 1=Always
  OpenNoTPMWipe    Y 1=Always
  OpenNoLongPP     Y 1=Always
  BatteryBypassPP  Y 1=Always
  I2C              Y 1=Always
  FlashRead        Y 1=Always
  OpenNoDevMode    Y 1=Always
  OpenFromUSB      Y 1=Always
  OverrideBatt     Y 1=Always
  AllowUnverifiedRo Y 1=Always
Capabilities are modified.
TPM:
Use 'ccd help' to print subcommands
	`

	expected := map[CCDCap]CCDCapState{
		UartGscRxAPTx:     CapAlways,
		UartGscTxAPRx:     CapAlways,
		UartGscRxECTx:     CapAlways,
		UartGscTxECRx:     CapAlways,
		UartGscRxFpmcuTx:  CapAlways,
		UartGscTxFpmcuRx:  CapAlways,
		FlashAP:           CapAlways,
		FlashEC:           CapIfOpened,
		OverrideWP:        CapAlways,
		RebootECAP:        CapAlways,
		GscFullConsole:    CapAlways,
		UnlockNoReboot:    CapAlways,
		UnlockNoShortPP:   CapAlways,
		OpenNoTPMWipe:     CapAlways,
		OpenNoLongPP:      CapAlways,
		BatteryBypassPP:   CapAlways,
		I2C:               CapAlways,
		FlashRead:         CapAlways,
		OpenNoDevMode:     CapAlways,
		OpenFromUSB:       CapAlways,
		OverrideBatt:      CapAlways,
		AllowUnverifiedRO: CapAlways,
	}

	testGetCCDCapabilities(t, input, expected)
}

func TestGetCCDCapabilities2(t *testing.T) {
	input := `
ccd
State: Locked
Password: none
Flags: 0x400005
Capabilities: 5555555555000000
  UartGscRxAPTx   Y 0=Default (Always)
  UartGscTxAPRx   Y 0=Default (Always)
  UartGscRxECTx   Y 0=Default (Always)
  UartGscTxECRx   Y 0=Default (IfOpened)
  FlashAP         Y 2=UnlessLocked
  FlashEC         Y 3=IfOpened
  OverrideWP      Y 1=Always
  RebootECAP      Y 1=Always
  GscFullConsole  Y 1=Always
  UnlockNoReboot  Y 1=Always
  UnlockNoShortPP Y 1=Always
  OpenNoTPMWipe   Y 1=Always
  OpenNoLongPP    Y 1=Always
  BatteryBypassPP Y 1=Always
  UpdateNoTPMWipe Y 1=Always
  I2C             Y 1=Always
  FlashRead       Y 1=Always
  OpenNoDevMode   Y 1=Always
  OpenFromUSB     Y 1=Always
  OverrideBatt    Y 1=Always
read_tpm_nvmem: object at 0x100a not found
[291.156501 Console unlock allowed]
read_tpm_nvmem: object at 0x1007 not found
TPM:
Capabilities are modified.
Use 'ccd help' to print subcommands
	`

	expected := map[CCDCap]CCDCapState{
		UartGscRxAPTx:   CapAlways,
		UartGscTxAPRx:   CapAlways,
		UartGscRxECTx:   CapAlways,
		UartGscTxECRx:   CapIfOpened,
		FlashAP:         CapUnlessLocked,
		FlashEC:         CapIfOpened,
		OverrideWP:      CapAlways,
		RebootECAP:      CapAlways,
		GscFullConsole:  CapAlways,
		UnlockNoReboot:  CapAlways,
		UnlockNoShortPP: CapAlways,
		OpenNoTPMWipe:   CapAlways,
		OpenNoLongPP:    CapAlways,
		BatteryBypassPP: CapAlways,
		UpdateNoTPMWipe: CapAlways,
		I2C:             CapAlways,
		FlashRead:       CapAlways,
		OpenNoDevMode:   CapAlways,
		OpenFromUSB:     CapAlways,
		OverrideBatt:    CapAlways,
	}

	testGetCCDCapabilities(t, input, expected)
}

func testGetCCDCapabilities(t *testing.T, input string, expected map[CCDCap]CCDCapState) {
	output, err := matchCCDCapabilities(input)
	if err != nil {
		t.Fatal("error processing CCD capabilities:", err)
	}

	if !reflect.DeepEqual(output, expected) {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", output, expected)
	}
}

func TestGettimeDT(t *testing.T) {
	input := `
		gettime
		Time: 0x0000000000a6ec69 = 10939.000 s
		Since reset: 0x0000000000a6ec69 = 10939.497 s
		Since deep sleep: 0x0000000000004a68 = 19.048 s
`
	expected := GSCTime{
		coldResetTime: 10939497000000,
		dsTime:        19048000000,
	}

	testExtractGSCTime(t, input, expected)
}

func TestGettimeH1(t *testing.T) {
	// These hex values do not match the base 10 value. They don't matter.
	input := `
		Time: 0x0000000000b98f67 = 67.890 s
		Since reset: 0x0000000000abcde = 12345.678 s
		Since deep sleep: 0x0000000000ade606 = 1.000 s
`
	expected := GSCTime{
		coldResetTime: 12345678000000,
		dsTime:        1000000000,
	}

	testExtractGSCTime(t, input, expected)
}

func testExtractGSCTime(t *testing.T, input string, expected GSCTime) {
	out, err := extractGSCTime(input)
	if err != nil {
		t.Fatal("error processing gettime info:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestUsbAdcInfoCr50(t *testing.T) {
	input := `PHY A`
	_, err := matchUsbAdcInfo(input)
	if err == nil {
		t.Fatal("expected match failure")
	}

	input = `PHY B`
	_, err = matchUsbAdcInfo(input)
	if err == nil {
		t.Fatal("expected match failure")
	}
}

func TestUsbAdcInfoDisconnected(t *testing.T) {
	input := `
ADC: disconnected
ADC: CC1 = 54 mV
ADC: CC2 = 53 mV
`
	expected := UsbAdcInfo{
		State: UsbDisconnected,
		Cc1Mv: 54,
		Cc2Mv: 53,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoSuzyQ(t *testing.T) {
	input := `
ADC: connected: SuzyQ
ADC: CC1 = 928 mV
ADC: CC2 = 497 mV
`
	expected := UsbAdcInfo{
		State: SuzyQConnected,
		Cc1Mv: 928,
		Cc2Mv: 497,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoSuzyQFlipped(t *testing.T) {
	input := `
ADC: connected: SuzyQFlipped
ADC: CC1 = 500 mV
ADC: CC2 = 928 mV
`
	expected := UsbAdcInfo{
		State: SuzyQFlippedConnected,
		Cc1Mv: 500,
		Cc2Mv: 928,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoServo(t *testing.T) {
	input := `
ADC: connected: Servo-src(Rp1A5/Rp3A0)
ADC: CC1 = 1706 mV
ADC: CC2 = 928 mV
`
	expected := UsbAdcInfo{
		State: ServoConnected,
		Cc1Mv: 1706,
		Cc2Mv: 928,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoServoFlipped(t *testing.T) {
	input := `
ADC: connected: Servo-src(Rp3A0/Rp1A5)
ADC: CC1 = 931 mV
ADC: CC2 = 1706 mV
	`
	expected := UsbAdcInfo{
		State: ServoFlippedConnected,
		Cc1Mv: 931,
		Cc2Mv: 1706,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoServoSink1(t *testing.T) {
	input := `
ADC: connected: Servo-snk(dut:RpUSB)
ADC: CC1 = 539 mV
ADC: CC2 = 536 mV
	`
	expected := UsbAdcInfo{
		State: ServoSink1Connected,
		Cc1Mv: 539,
		Cc2Mv: 536,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoServoSink2(t *testing.T) {
	input := `
ADC: connected: Servo-snk(dut:Rp1A5)
ADC: CC1 = 1009 mV
ADC: CC2 = 1008 mV
	`
	expected := UsbAdcInfo{
		State: ServoSink2Connected,
		Cc1Mv: 1009,
		Cc2Mv: 1008,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func TestUsbAdcInfoServoSink3(t *testing.T) {
	input := `
ADC: connected: Servo-snk(dut:Rp3A0)
ADC: CC1 = 1827 mV
ADC: CC2 = 1828 mV
	`
	expected := UsbAdcInfo{
		State: ServoSink3Connected,
		Cc1Mv: 1827,
		Cc2Mv: 1828,
	}

	testMatchUsbAdcInfo(t, input, expected)
}

func testMatchUsbAdcInfo(t *testing.T, input string, expected UsbAdcInfo) {
	out, err := matchUsbAdcInfo(input)
	if err != nil {
		t.Fatal("error processing USB ADC info:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestRmaChallenge1(t *testing.T) {
	input := `
rma_auth
[20.834407 tpm_reset_now: done]
generated challenge:

AH9CDTMQXHPF8EHZDW6KV2YGSFT262DMZU9WTQXGFWR7MUXXZWQFW9KQW59L99999W2WECCCTS6ELUNM

`
	expected := "AH9CDTMQXHPF8EHZDW6KV2YGSFT262DMZU9WTQXGFWR7MUXXZWQFW9KQW59L99999W2WECCCTS6ELUNM"
	testMatchRmaChallenge(t, input, expected)
}

func TestRmaChallenge2(t *testing.T) {
	input := `
rma_auth
RMA Auth error 0x504
`
	expected := ""
	testMatchRmaChallenge(t, input, expected)
}

func TestRmaChallenge3(t *testing.T) {
	input := `
rma_auth
Must wait to generate another code.
Command 'rma_auth' failed: Unknown
`
	expected := ""
	testMatchRmaChallenge(t, input, expected)
}

func TestRmaChallenge4(t *testing.T) {
	input := `
rma_auth
[10.278938 tpm_reset_now(0)]
[10.282257 tpm_init]
[10.283695 Skipping commit]
[10.284729 tpm_manufactured: NOT manufactured]
[10.289505 Skipping commit]
[10.290397 get_decrypted_eps: getting eps]
[10.322429 Skipping commit]
[10.323318 tpm_endorse: RSA cert install success]
[10.331787 Skipping commit]
[10.332656 tpm_endorse: ECC cert install success]
[10.333755 Skipping commit]
[10.334430 endorsement_complete: SUCCESS]
[10.335258 Endorsement succeeded (0)]
[10.348663 tpm_reset_now: done]
generated challenge:

AH3PCEXSH9MY7RMRSA5B3ZEFSVUTP8P9ZDHEXVNNFGNHXQ69JR7CWD6BFB9Y99999SG763A7QWHH52SR

>
`
	expected := "AH3PCEXSH9MY7RMRSA5B3ZEFSVUTP8P9ZDHEXVNNFGNHXQ69JR7CWD6BFB9Y99999SG763A7QWHH52SR"
	testMatchRmaChallenge(t, input, expected)
}

func testMatchRmaChallenge(t *testing.T, input, expected string) {
	out, err := matchRmaChallenge(input)
	if err != nil {
		t.Fatal("error processing rma auth:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestH1ChipDetectDetachable1(t *testing.T) {
	input := `cr50 B2-C`
	expected := SKUH1Clamshell
	testFindH1ChipSKU(t, input, expected)
}

func TestH1ChipDetectDetachable2(t *testing.T) {
	input := `cr50 B2-D`
	expected := SKUH1Detachable
	testFindH1ChipSKU(t, input, expected)
}
func TestH1ChipDetectDetachable3(t *testing.T) {
	input := `
Reset flags: 0x00000140 (hibernate wake-pin)
Reset count: 1
Chip:        g cr50 B2-C
RO keyid:    0xaa66150f
RW keyid:    0x87b73b67
DEV_ID:      0x00000000 0x00000000
Rollback:    1/1/2 4/4/4
TPM MODE:    enabled (0)
Key Ladder:  prod
`
	expected := SKUH1Clamshell
	testFindH1ChipSKU(t, input, expected)
}

func testFindH1ChipSKU(t *testing.T, input string, expected ChipSKU) {
	chip, err := FindH1ChipSKU(input)
	if err != nil {
		t.Fatal("error processing input:", err)
	}
	if expected != chip {
		t.Fatalf("chip mismatch:\ngot      %v\nexpected %v", chip, expected)
	}
}
