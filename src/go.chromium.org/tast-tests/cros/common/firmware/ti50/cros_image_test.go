// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
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
		RwA:   RwInfo{Empty: false, Active: true, Version: "0.24.61", Branch: ToT, VersionStr: "ti50_common_tot:v0.0.909-efe9eb23"},
		RwB:   RwInfo{Empty: true},
		BID:   ImageBID{Empty: false, Type: 0x46464646, Mask: 0, Flags: 0x10},
		Build: BuildInfo{Branch: ToT, VersionStr: "ti50_common_tot:v0.0.909-efe9eb23"},
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
		RwA:   RwInfo{Empty: false, Active: true, Version: "0.24.20", Branch: Unknown, VersionStr: "ti50_common:v0.0.2888-c2eeb17e"},
		RwB:   RwInfo{Empty: false, Active: false, Debug: true, Version: "1.24.30", Branch: ToT, VersionStr: "DBG/ti50_common_tot:v0.0.360+ef9becfc"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: Unknown, Debug: false, VersionStr: "ti50_common:v0.0.2888-c2eeb17e"},
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
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.24.20", Branch: Unknown, VersionStr: "ti50_common:v0.0.2888-c2eeb17e"},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.24.30", Branch: ToT, VersionStr: "DBG/ti50_common_tot:v0.0.360+ef9becfc"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: ToT, Debug: true, VersionStr: "DBG/ti50_common_tot:v0.0.360+ef9becfc"},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandTi50Processor4(t *testing.T) {
	input := `
RO_A:  * 0.0.52/d1072955
RO_B:    Empty
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
		RoA:   RoInfo{Active: true, Version: "0.0.52", ImageCheck: "d1072955"},
		RoB:   RoInfo{},
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.24.20", Branch: Unknown, VersionStr: "ti50_common:v0.0.2888-c2eeb17e"},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.24.30", Branch: ToT, VersionStr: "DBG/ti50_common_tot:v0.0.360+ef9becfc"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: ToT, Debug: true, VersionStr: "DBG/ti50_common_tot:v0.0.360+ef9becfc"},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandTi50Processor5(t *testing.T) {
	input := `
RO_A:  * 0.0.52/d1072955
RO_B:    Empty
RW_A:    0.24.20/ti50_common:v0.0.2888-c2eeb17e
RW_B:  * 1.24.30/DBG/ti50a_common_tot:v0.0.360+ef9becfc
BID A:   00000000:00000000:00000000 Yes
BID B:   00000000:00000000:00000000 Yes
Build:   DBG/ti50a_common_tot:v0.0.360+ef9becfc
         libtock-rs:v0.0.915-10cf494
         tock:v0.0.9632-b3a7d9d57
         ms-tpm-20-ref:v0.0.306-ba9d73d
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: true, Version: "0.0.52", ImageCheck: "d1072955"},
		RoB:   RoInfo{},
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.24.20", Branch: Unknown, VersionStr: "ti50_common:v0.0.2888-c2eeb17e"},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.24.30", Branch: ToT, VersionStr: "DBG/ti50a_common_tot:v0.0.360+ef9becfc"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: ToT, Debug: true, VersionStr: "DBG/ti50a_common_tot:v0.0.360+ef9becfc"},
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
		RwA:   RwInfo{Empty: false, Active: true, Version: "0.3.22", Branch: Unknown, VersionStr: "cr50_v1.9308_26_0.596-e6b91d6"},
		RwB:   RwInfo{Empty: false, Active: false, Version: "0.3.22", Branch: Unknown, VersionStr: "cr50_v1.9308_26_0.596-e6b91d6"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: Unknown, VersionStr: "cr50_v1.9308_26_0.596-e6b91d6"},
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
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.6.211", Branch: PrePvt, VersionStr: "cr50_v3.94_pp.256-f6119fcacf"},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.6.205", Branch: ToT, VersionStr: "DBG/cr50_v2.0.3597-2b7751b89f"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: ToT, Debug: true, VersionStr: "DBG/cr50_v2.0.3597-2b7751b89f"},
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
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.5.215", Branch: MP, VersionStr: "cr50_v4.11_mp.258-bcc62291cf"},
		RwB:   RwInfo{Empty: false, Active: true, Version: "0.6.230", Branch: PrePvt, VersionStr: "cr50_v3.94_pp.259-e20fb86af1"},
		BID:   ImageBID{Empty: false, Type: 0x46464646, Mask: 0, Flags: 0x10},
		Build: BuildInfo{Branch: PrePvt, Debug: false, VersionStr: "cr50_v3.94_pp.259-e20fb86af1"},
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
		RwB:   RwInfo{Empty: false, Active: true, Version: "0.6.230", Branch: PrePvt, VersionStr: "cr50_v3.94_pp.259-e20fb86af1"},
		BID:   ImageBID{Empty: false, Type: 0x46464646, Mask: 0, Flags: 0x10},
		Build: BuildInfo{Branch: PrePvt, Debug: false, VersionStr: "cr50_v3.94_pp.259-e20fb86af1"},
	}

	testVersionInfoMatcher(t, input, expected)
}

func TestVersionCommandCr50Processor5(t *testing.T) {
	input := `
Chip:    g cr50 B2-C
Board:   0
RO_A:    0.0.11/bc74f7dc
RO_B:  * 0.0.12/9eb618de
RW_A:    0.5.12/cr50_v4.11_28_efi.0-8441619945
RW_B:  * 1.6.250/DBG/cr50_v4.08_pp.61+a50884addc
BID A:   00000000:00000000:00000000 Yes
BID B:   00000000:00000000:00000000 Yes
Build:   1.6.250/DBG/cr50_v4.08_pp.61+a50884addc
         tpm2:v1.9308_26_0.93-d6c7c5f
         pinweaver:v0.0.156-4ec2204
`
	expected := VersionCommandInfo{
		RoA:   RoInfo{Active: false, Version: "0.0.11", ImageCheck: "bc74f7dc"},
		RoB:   RoInfo{Active: true, Version: "0.0.12", ImageCheck: "9eb618de"},
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.5.12", Branch: EFI, VersionStr: "cr50_v4.11_28_efi.0-8441619945"},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.6.250", Branch: PrePvt, VersionStr: "DBG/cr50_v4.08_pp.61+a50884addc"},
		BID:   ImageBID{Empty: false, Type: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: PrePvt, Debug: true, VersionStr: "DBG/cr50_v4.08_pp.61+a50884addc"},
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

func TestValidateVersionInfoRunningSameRW(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: true, Version: "0.3.22", Branch: Unknown},
		RwB: RwInfo{Empty: false, Active: false, Version: "0.3.22", Branch: Unknown},
	}

	// Verify checking the active slot works
	testValidateVersionInfo(t, input, "0.3.22", false, true)
	// Verify checking both slots works
	testValidateVersionInfo(t, input, "0.3.22", false, true)
}

func TestValidateVersionInfoRunningDifferentRW(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: true, Version: "0.6.211", Branch: PrePvt},
		RwB: RwInfo{Empty: false, Active: false, Version: "1.6.205", Branch: ToT},
	}
	testValidateVersionInfo(t, input, "0.6.211", false, true)
	// Verify checking both slots doesn't match
	testValidateVersionInfo(t, input, "0.6.211", true, false)
	// Verify it doesn't match RW_B
	testValidateVersionInfo(t, input, "1.6.205", false, false)
}

func TestValidateVersionInfoDebugMismatch(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: false, Version: "0.6.211", Branch: PrePvt},
		RwB: RwInfo{Empty: false, Active: true, Debug: true, Version: "0.6.211", Branch: PrePvt},
	}
	testValidateVersionInfo(t, input, "0.6.211", false, true)
	// Verify checking both slots doesn't match
	testValidateVersionInfo(t, input, "0.6.211", true, false)
	// Verify changing the RwA Debug status makes the versions match
	input.RwA.Debug = true
	testValidateVersionInfo(t, input, "0.6.211", true, true)
}
func TestValidateVersionInfoVersionStrMismatch(t *testing.T) {
	input := VersionCommandInfo{
		RwA: RwInfo{Empty: false, Active: false, Version: "0.6.211", VersionStr: "gsc_RWA", Branch: PrePvt},
		RwB: RwInfo{Empty: false, Active: true, Version: "0.6.211", VersionStr: "gsc_RWB", Branch: PrePvt},
	}
	testValidateVersionInfo(t, input, "0.6.211", false, true)
	// Verify checking both slots doesn't match
	testValidateVersionInfo(t, input, "0.6.211", true, false)
	// Verify changing the RwA Debug status makes the versions match
	input.RwA.VersionStr = "gsc_RWB"
	testValidateVersionInfo(t, input, "0.6.211", true, true)
}

func testValidateVersionInfo(t *testing.T, input VersionCommandInfo, expectedVersion string, checkBothSlots, expectMatch bool) {
	matches := ValidateVersionInfo(input, expectedVersion, checkBothSlots)
	if expectMatch && !matches {
		t.Fatalf("unexpected mismatch: %+v did not match %s bothSlots(%t)", input, expectedVersion, checkBothSlots)
	}
	if !expectMatch && matches {
		t.Fatalf("unexpected match: %+v matched %s bothSlots(%t)", input, expectedVersion, checkBothSlots)
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
		t.Fatalf("output mismatch:got      %v\nexpected %v", out, expected)
	}
}

func TestRwInfoMatcherInvalidInput(t *testing.T) {
	if _, err := matchRwInfo(`RW_B:    Empty`, SlotA); err == nil {
		t.Fatal("expected error")
	}
}

func TestRwInfoMatcher1(t *testing.T) {
	input := `RW_A:  * 0.24.61/ti50_common_tot:v0.0.909-efe9eb23`
	expected := RwInfo{Active: true, Empty: false, Version: "0.24.61", Branch: ToT, VersionStr: "ti50_common_tot:v0.0.909-efe9eb23"}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher2(t *testing.T) {
	input := `RW_B:    Empty`
	expected := RwInfo{Empty: true}
	testRwInfoMatcher(t, input, SlotB, expected)
}

func TestRwInfoMatcher3(t *testing.T) {
	input := `RW_A:  * 0.24.60/ti50_common_prepvt-15086.B:v0.0.782-aca516e7`
	expected := RwInfo{Active: true, Empty: false, Version: "0.24.60", Branch: PrePvt, VersionStr: "ti50_common_prepvt-15086.B:v0.0.782-aca516e7"}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher4(t *testing.T) {
	input := `RW_A:  * 0.23.60/ti50_common_mp-15224.B:v0.0.729-2ab3d1fb`
	expected := RwInfo{Active: true, Empty: false, Version: "0.23.60", Branch: MP, VersionStr: "ti50_common_mp-15224.B:v0.0.729-2ab3d1fb"}
	testRwInfoMatcher(t, input, SlotA, expected)
}

func TestRwInfoMatcher5(t *testing.T) {
	input := `RW_A:  * 0.3.22/cr50_v1.9308_26_0.596-e6b91d6`
	expected := RwInfo{Active: true, Empty: false, Version: "0.3.22", Branch: Unknown, VersionStr: "cr50_v1.9308_26_0.596-e6b91d6"}
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

func TestImageBIDMatcher1(t *testing.T) {
	input := `BID A:   46464646:00000000:00000010 Yes`
	expected := ImageBID{Type: 0x46464646, Mask: 0, Flags: 0x10}
	testImageBIDMatcher(t, input, SlotA, expected)
}

func TestImageBIDMatcher2(t *testing.T) {
	input := `BID B:   46464646:00000000:00000010 Yes`
	expected := ImageBID{Type: 0x46464646, Mask: 0, Flags: 0x10}
	testImageBIDMatcher(t, input, SlotB, expected)
}

func testImageBIDMatcher(t *testing.T, input string, slot GscSlot, expected ImageBID) {
	out, err := matchImageBID(input, slot)
	if err != nil {
		t.Fatal("error processing bid info:", err)
	}
	if out != expected {
		t.Fatal("output mismatch")
	}
}

func TestBuildInfoMatcher1(t *testing.T) {
	input := `Build:   ti50_common_tot:v0.0.909-efe9eb23`
	expected := BuildInfo{Branch: ToT, VersionStr: "ti50_common_tot:v0.0.909-efe9eb23"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher2(t *testing.T) {
	input := `Build:   ti50_common_prepvt-15086.B:v0.0.782-aca516e7`
	expected := BuildInfo{Branch: PrePvt, VersionStr: "ti50_common_prepvt-15086.B:v0.0.782-aca516e7"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher3(t *testing.T) {
	input := `Build:   ti50_common_mp-15224.B:v0.0.729-2ab3d1fb`
	expected := BuildInfo{Branch: MP, VersionStr: "ti50_common_mp-15224.B:v0.0.729-2ab3d1fb"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher4(t *testing.T) {
	input := `Build:   0.3.22/cr50_v1.9308_26_0.596-e6b91d6`
	expected := BuildInfo{Branch: Unknown, VersionStr: "cr50_v1.9308_26_0.596-e6b91d6"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher5(t *testing.T) {
	input := `Build:   0.5.12/cr50_v4.11_28_efi.0-8441619945`
	expected := BuildInfo{Branch: EFI, VersionStr: "cr50_v4.11_28_efi.0-8441619945"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher6(t *testing.T) {
	input := `Build:   0.6.241/cr50_v4.08_pp.61-a50884addc`
	expected := BuildInfo{Branch: PrePvt, VersionStr: "cr50_v4.08_pp.61-a50884addc"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher7(t *testing.T) {
	input := `Build:   ti50_common:v0.0.2888-c2eeb17e`
	expected := BuildInfo{Branch: Unknown, VersionStr: "ti50_common:v0.0.2888-c2eeb17e"}
	testBuildInfoMatcher(t, input, expected)
}

func TestBuildInfoMatcher8(t *testing.T) {
	input := `Build:   DBG/ti50_common_tot:v0.0.360+ef9becfc`
	expected := BuildInfo{Branch: ToT, Debug: true, VersionStr: "DBG/ti50_common_tot:v0.0.360+ef9becfc"}
	testBuildInfoMatcher(t, input, expected)
}
func TestBuildInfoMatcher9(t *testing.T) {
	input := `Build:   1.6.205/DBG/cr50_v2.0.3597-2b7751b89f`
	expected := BuildInfo{Branch: ToT, Debug: true, VersionStr: "DBG/cr50_v2.0.3597-2b7751b89f"}
	testBuildInfoMatcher(t, input, expected)
}

func testBuildInfoMatcher(t *testing.T, input string, expected BuildInfo) {
	out, err := matchBuildInfo(input)
	if err != nil {
		t.Fatal("error processing bid info:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:got      %v\nexpected %v", out, expected)
	}
}

func TestCCDCapabilities1(t *testing.T) {
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

	testCCDCapabilities(t, input, expected)
}

func TestCCDCapabilities2(t *testing.T) {
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

	testCCDCapabilities(t, input, expected)
}

func testCCDCapabilities(t *testing.T, input string, expected map[CCDCap]CCDCapState) {
	output, err := matchCCDCapabilities(input)
	if err != nil {
		t.Fatal("error processing CCD capabilities:", err)
	}

	if !reflect.DeepEqual(output, expected) {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", output, expected)
	}
}

func TestTimeDT(t *testing.T) {
	input := `
		gettime
		Time: 0x0000000000a6ec69 = 10939.000 s
		Since reset: 0x0000000000a6ec69 = 10939.497 s
		Since deep sleep: 0x0000000000004a68 = 19.048 s
`
	expected := GSCTime{
		ColdResetTime: 10939497000000,
		DeepSleepTime: 19048000000,
	}

	testExtractGSCTime(t, input, expected)
}

func TestTimeCr501(t *testing.T) {
	input := `
		gettime
		Time: 0x00000000001ea5ff = 2.008575 s
		since cold_reset: 1683 s
`
	expected := GSCTime{
		ColdResetTime: 1683000000000,
		DeepSleepTime: 2008575000,
	}

	testExtractGSCTime(t, input, expected)
}
func TestTimeCr502(t *testing.T) {
	input := `
		gettime
		Time: 0x00000000009864dd = 9.987293 s
		since cold_reset: 9 s
`
	expected := GSCTime{
		ColdResetTime: 9000000000,
		DeepSleepTime: 9987293000,
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

func TestUSBADCInfoCr50(t *testing.T) {
	input := `PHY A`
	_, err := matchUSBADCInfo(input)
	if err == nil {
		t.Fatal("expected match failure")
	}

	input = `PHY B`
	_, err = matchUSBADCInfo(input)
	if err == nil {
		t.Fatal("expected match failure")
	}
}

func TestUSBADCInfoDisconnected(t *testing.T) {
	input := `
ADC: disconnected
ADC: CC1 = 54 mV
ADC: CC2 = 53 mV
`
	expected := USBADCInfo{
		State: UsbDisconnected,
		Cc1Mv: 54,
		Cc2Mv: 53,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoSuzyQ(t *testing.T) {
	input := `
ADC: connected: SuzyQ
ADC: CC1 = 928 mV
ADC: CC2 = 497 mV
`
	expected := USBADCInfo{
		State: SuzyQConnected,
		Cc1Mv: 928,
		Cc2Mv: 497,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoSuzyQFlipped(t *testing.T) {
	input := `
ADC: connected: Servo-src(RpUSB/Rp1A5)
ADC: CC1 = 500 mV
ADC: CC2 = 928 mV
`
	expected := USBADCInfo{
		State: Servo1A5FlippedConnected,
		Cc1Mv: 500,
		Cc2Mv: 928,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoServo(t *testing.T) {
	input := `
ADC: connected: Servo-src(Rp3A0/Rp1A5)
ADC: CC1 = 1706 mV
ADC: CC2 = 928 mV
`
	expected := USBADCInfo{
		State: ServoConnected,
		Cc1Mv: 1706,
		Cc2Mv: 928,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoServoFlipped(t *testing.T) {
	input := `
ADC: connected: Servo-src(Rp1A5/Rp3A0)
ADC: CC1 = 931 mV
ADC: CC2 = 1706 mV
	`
	expected := USBADCInfo{
		State: ServoFlippedConnected,
		Cc1Mv: 931,
		Cc2Mv: 1706,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoServoSink1(t *testing.T) {
	input := `
ADC: connected: Servo-snk(dut:RpUSB)
ADC: CC1 = 539 mV
ADC: CC2 = 536 mV
	`
	expected := USBADCInfo{
		State: ServoSink1Connected,
		Cc1Mv: 539,
		Cc2Mv: 536,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoServoSink2(t *testing.T) {
	input := `
ADC: connected: Servo-snk(dut:Rp1A5)
ADC: CC1 = 1009 mV
ADC: CC2 = 1008 mV
	`
	expected := USBADCInfo{
		State: ServoSink2Connected,
		Cc1Mv: 1009,
		Cc2Mv: 1008,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func TestUSBADCInfoServoSink3(t *testing.T) {
	input := `
ADC: connected: Servo-snk(dut:Rp3A0)
ADC: CC1 = 1827 mV
ADC: CC2 = 1828 mV
	`
	expected := USBADCInfo{
		State: ServoSink3Connected,
		Cc1Mv: 1827,
		Cc2Mv: 1828,
	}

	testMatchUSBADCInfo(t, input, expected)
}

func testMatchUSBADCInfo(t *testing.T, input string, expected USBADCInfo) {
	out, err := matchUSBADCInfo(input)
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

func TestH1ChipDetectClamshell(t *testing.T) {
	input := `cr50 B2-C`
	expected := SKUH1Clamshell
	testFindChipSKU(t, input, expected)
}

func TestH1ChipDetectDetachable2(t *testing.T) {
	input := `cr50 B2-D`
	expected := SKUH1Detachable
	testFindChipSKU(t, input, expected)
}
func TestH1ChipDetectDT(t *testing.T) {
	input := `Ti50 D3C1`
	expected := SKUDT
	testFindChipSKU(t, input, expected)
}
func TestH1ChipDetectDT2(t *testing.T) {
	input := `Ti50 other sku`
	expected := SKUDT
	testFindChipSKU(t, input, expected)
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
	testFindChipSKU(t, input, expected)
}

func testFindChipSKU(t *testing.T, input string, expected ChipSKU) {
	chip := FindChipSKU(input)
	if expected != chip {
		t.Fatalf("chip mismatch:\ngot      %v\nexpected %v", chip, expected)
	}
}

func TestTi50Sysinfo1(t *testing.T) {
	input := `
Reset flags: 0x00000001 (Cold)
Reset count: 1
Breadcrumbs: 0x0000000000001234
Chip:        g Ti50 D3C1
RO keyid:    0xc7d40497
RW keyid:    0xfba25ca9
DEV_ID:      0x14820030 0x4c2ac261
Rollback:    0.3/0.3/0.3 128.128/?.?/4.4
TPM MODE:    disabled (3)
Key Ladder:  prod
EK Cert:     Compliant
`
	expectedMap := make(map[string]string)
	expectedMap["resetFlags"] = "00000001"
	expectedMap["resetCount"] = "1"
	expectedMap["breadcrumbs"] = "0000000000001234"
	expectedMap["chipName"] = "Ti50"
	expectedMap["chipSKU"] = "D3C1"
	expectedMap["roKeyid"] = "0xc7d40497"
	expectedMap["rwKeyid"] = "0xfba25ca9"
	expectedMap["devid"] = "0x14820030 0x4c2ac261"
	expectedMap["roRollback"] = "0.3/0.3/0.3"
	expectedMap["rwRollback"] = "128.128/?.?/4.4"
	expectedMap["tpmMode"] = "disabled"
	expectedMap["tpmModeStatus"] = "3"
	expectedMap["keyladder"] = "prod"
	expectedMap["ekCert"] = "Compliant"
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.ResetFlags = 0x00000001
	expected.OriginalResetFlags = 0x00000001
	expected.ResetCount = 1
	expected.Breadcrumbs = "0000000000001234"
	expected.ChipName = "Ti50"
	expected.ChipSKU = SKUDT
	expected.ROKeyid = "0xc7d40497"
	expected.RWKeyid = "0xfba25ca9"
	expected.Devid = "0x14820030 0x4c2ac261"
	expected.RORollback = "0.3/0.3/0.3"
	expected.RWRollback = "128.128/?.?/4.4"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 128, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: InvalidBits, Valid: false},
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
	}
	expected.TpmMode = "disabled"
	expected.TpmModeStatus = 3
	expected.Keyladder = "prod"
	expected.TpmEnabled = false
	expected.ProdKeyladder = true
	expected.InFactoryMode = false
	expected.FactoryModeValid = true
	expected.EKCert = "Compliant"

	testSysinfo(t, input, expectedMap, expected)
}

func TestTi50Sysinfo2(t *testing.T) {
	input := `
sysinfo
Reset flags: 0x00000020 (Cold)
Reset count: 2
Breadcrumbs: 0x0000000123491234
Chip:        g Ti50 D3C1
RO keyid:    0xc7d40497
RW keyid:    0xfba25ca9
DEV_ID:      0x14820030 0x4c2ac261
Rollback:    3/3/3 4/?/5
TPM MODE:    disabled (3)
Key Ladder:  prod
EK Cert:     Compliant
Chip factory mode.
`
	expectedMap := make(map[string]string)
	expectedMap["resetFlags"] = "00000020"
	expectedMap["resetCount"] = "2"
	expectedMap["chipName"] = "Ti50"
	expectedMap["chipSKU"] = "D3C1"
	expectedMap["breadcrumbs"] = "0000000123491234"
	expectedMap["roKeyid"] = "0xc7d40497"
	expectedMap["rwKeyid"] = "0xfba25ca9"
	expectedMap["devid"] = "0x14820030 0x4c2ac261"
	expectedMap["roRollback"] = "3/3/3"
	expectedMap["rwRollback"] = "4/?/5"
	expectedMap["tpmMode"] = "disabled"
	expectedMap["tpmModeStatus"] = "3"
	expectedMap["keyladder"] = "prod"
	expectedMap["ekCert"] = "Compliant"
	expectedMap["factoryMode"] = "Chip factory mode."
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.ResetFlags = 0x00000020
	expected.OriginalResetFlags = 0x00000020
	expected.ResetCount = 2
	expected.ChipName = "Ti50"
	expected.ChipSKU = SKUDT
	expected.Breadcrumbs = "0000000123491234"
	expected.ROKeyid = "0xc7d40497"
	expected.RWKeyid = "0xfba25ca9"
	expected.Devid = "0x14820030 0x4c2ac261"
	expected.RORollback = "3/3/3"
	expected.RWRollback = "4/?/5"
	expected.RWRollbackBits = SysinfoRollbackBits{
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
		SlotA: SysinfoRollbackSlot{Bits: InvalidBits, Valid: false},
		SlotB: SysinfoRollbackSlot{Bits: 5, Valid: true},
	}
	expected.TpmMode = "disabled"
	expected.TpmModeStatus = 3
	expected.Keyladder = "prod"
	expected.TpmEnabled = false
	expected.ProdKeyladder = true
	expected.InFactoryMode = true
	expected.FactoryModeValid = true
	expected.EKCert = "Compliant"

	testSysinfo(t, input, expectedMap, expected)
}

func TestTi50Sysinfo3(t *testing.T) {
	input := `
> sysinfo
sysinfo
Reset flags: 0x00000001 (Cold)
Reset count: 0
Breadcrumbs: 0x0000000000001234
Chip:        g Ti50 NTZ1
RO keyid:         0x0
RW keyid:         0x0
DEV_ID:      0x315a0000 0x30430054
Rollback:    ?.?/?.?/?.? ?.?/?.?/?.?
TPM MODE:    disabled (3)
Key Ladder:  dev
EK Cert:     NotFound
`
	expectedMap := make(map[string]string)
	expectedMap["resetFlags"] = "00000001"
	expectedMap["resetCount"] = "0"
	expectedMap["chipName"] = "Ti50"
	expectedMap["chipSKU"] = "NTZ1"
	expectedMap["breadcrumbs"] = "0000000000001234"
	expectedMap["roKeyid"] = "0x0"
	expectedMap["rwKeyid"] = "0x0"
	expectedMap["devid"] = "0x315a0000 0x30430054"
	expectedMap["roRollback"] = "?.?/?.?/?.?"
	expectedMap["rwRollback"] = "?.?/?.?/?.?"
	expectedMap["tpmMode"] = "disabled"
	expectedMap["tpmModeStatus"] = "3"
	expectedMap["keyladder"] = "dev"
	expectedMap["ekCert"] = "NotFound"
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.ResetFlags = 0x00000001
	expected.OriginalResetFlags = 0x00000001
	expected.ResetCount = 0
	expected.ChipName = "Ti50"
	expected.ChipSKU = SKUDT
	expected.Breadcrumbs = "0000000000001234"
	expected.ROKeyid = "0x0"
	expected.RWKeyid = "0x0"
	expected.Devid = "0x315a0000 0x30430054"
	expected.RORollback = "?.?/?.?/?.?"
	expected.RWRollback = "?.?/?.?/?.?"
	expected.RWRollbackBits = SysinfoRollbackBits{
		Flash: SysinfoRollbackSlot{Bits: 0, Valid: false},
		SlotA: SysinfoRollbackSlot{Bits: InvalidBits, Valid: false},
		SlotB: SysinfoRollbackSlot{Bits: InvalidBits, Valid: false},
	}
	expected.TpmMode = "disabled"
	expected.TpmModeStatus = 3
	expected.Keyladder = "dev"
	expected.TpmEnabled = false
	expected.ProdKeyladder = false
	expected.InFactoryMode = false
	expected.FactoryModeValid = true
	expected.EKCert = "NotFound"

	testSysinfo(t, input, expectedMap, expected)
}

func TestCr50Sysinfo1(t *testing.T) {
	input := `
Reset flags: 0x00000008 (power-on)
Reset count: 1
Chip:        g cr50 B2-D
RO keyid:    0xaa66150f
RW keyid:    0x87b73b67
DEV_ID:      0x12345678 0x12345678
Rollback:    1/1/2 4/4/2
TPM MODE:    disabled (2)
Key Ladder:  dev
`
	expectedMap := make(map[string]string)
	expectedMap["breadcrumbs"] = ""
	expectedMap["resetFlags"] = "00000008"
	expectedMap["resetCount"] = "1"
	expectedMap["chipName"] = "cr50"
	expectedMap["chipSKU"] = "B2-D"
	expectedMap["rwKeyid"] = "0x87b73b67"
	expectedMap["roKeyid"] = "0xaa66150f"
	expectedMap["devid"] = "0x12345678 0x12345678"
	expectedMap["roRollback"] = "1/1/2"
	expectedMap["rwRollback"] = "4/4/2"
	expectedMap["tpmMode"] = "disabled"
	expectedMap["tpmModeStatus"] = "2"
	expectedMap["keyladder"] = "dev"
	expectedMap["ekCert"] = ""
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.Breadcrumbs = ""
	expected.OriginalResetFlags = 0x00000008
	expected.ResetFlags = GscResetFlagPowerOn
	expected.ResetCount = 1
	expected.ChipName = "cr50"
	expected.ChipSKU = SKUH1Detachable
	expected.RWKeyid = "0x87b73b67"
	expected.ROKeyid = "0xaa66150f"
	expected.Devid = "0x12345678 0x12345678"
	expected.RORollback = "1/1/2"
	expected.RWRollback = "4/4/2"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 4, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: 2, Valid: true},
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
	}
	expected.TpmMode = "disabled"
	expected.TpmModeStatus = 2
	expected.Keyladder = "dev"
	expected.TpmEnabled = false
	expected.ProdKeyladder = false
	expected.FactoryModeValid = false
	expected.InFactoryMode = false
	expected.EKCert = ""

	testSysinfo(t, input, expectedMap, expected)
}

func TestCr50Sysinfo2(t *testing.T) {
	input := `
Reset flags: 0x00000800 (hard)
Reset count: 1
Chip:        g cr50 B2-C
RO keyid:    0xaa66150f
RW keyid:    0x87b73b67
DEV_ID:      0x12345678 0x12345678
Rollback:    1/1/2 4/4/4
TPM MODE:    enabled (0)
Key Ladder:  prod
`
	expectedMap := make(map[string]string)

	expectedMap["breadcrumbs"] = ""
	expectedMap["resetFlags"] = "00000800"
	expectedMap["resetCount"] = "1"
	expectedMap["chipName"] = "cr50"
	expectedMap["chipSKU"] = "B2-C"
	expectedMap["roKeyid"] = "0xaa66150f"
	expectedMap["rwKeyid"] = "0x87b73b67"
	expectedMap["devid"] = "0x12345678 0x12345678"
	expectedMap["roRollback"] = "1/1/2"
	expectedMap["rwRollback"] = "4/4/4"
	expectedMap["tpmMode"] = "enabled"
	expectedMap["tpmModeStatus"] = "0"
	expectedMap["keyladder"] = "prod"
	expectedMap["ekCert"] = ""
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.Breadcrumbs = ""
	expected.ResetFlags = GscResetFlagHard
	expected.OriginalResetFlags = 0x00000800
	expected.ResetCount = 1
	expected.ChipName = "cr50"
	expected.ChipSKU = SKUH1Clamshell
	expected.ROKeyid = "0xaa66150f"
	expected.RWKeyid = "0x87b73b67"
	expected.Devid = "0x12345678 0x12345678"
	expected.RORollback = "1/1/2"
	expected.RWRollback = "4/4/4"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 4, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: 4, Valid: true},
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
	}
	expected.TpmMode = "enabled"
	expected.TpmModeStatus = 0
	expected.Keyladder = "prod"
	expected.TpmEnabled = true
	expected.ProdKeyladder = true
	expected.FactoryModeValid = false
	expected.InFactoryMode = false
	expected.EKCert = ""

	testSysinfo(t, input, expectedMap, expected)
}

func TestCr50Sysinfo3(t *testing.T) {
	input := `
Reset flags: 0x00000800 (hard)
Reset count: 1
Chip:        g cr50 B2-C
RO keyid:    0xaa66150f
RW keyid:    0x87b73b67
DEV_ID:      0x12345678 0x12345678
Rollback:    1/1/2 4/4/4
TPM MODE:    enabled (0)
Key Ladder:  prod
`
	expectedMap := make(map[string]string)

	expectedMap["breadcrumbs"] = ""
	expectedMap["resetFlags"] = "00000800"
	expectedMap["resetCount"] = "1"
	expectedMap["chipName"] = "cr50"
	expectedMap["chipSKU"] = "B2-C"
	expectedMap["roKeyid"] = "0xaa66150f"
	expectedMap["rwKeyid"] = "0x87b73b67"
	expectedMap["devid"] = "0x12345678 0x12345678"
	expectedMap["roRollback"] = "1/1/2"
	expectedMap["rwRollback"] = "4/4/4"
	expectedMap["tpmMode"] = "enabled"
	expectedMap["tpmModeStatus"] = "0"
	expectedMap["keyladder"] = "prod"
	expectedMap["ekCert"] = ""
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.Breadcrumbs = ""
	expected.ResetFlags = GscResetFlagHard
	expected.OriginalResetFlags = 0x00000800
	expected.ResetCount = 1
	expected.ChipName = "cr50"
	expected.ChipSKU = SKUH1Clamshell
	expected.ROKeyid = "0xaa66150f"
	expected.RWKeyid = "0x87b73b67"
	expected.Devid = "0x12345678 0x12345678"
	expected.RORollback = "1/1/2"
	expected.RWRollback = "4/4/4"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 4, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: 4, Valid: true},
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
	}
	expected.TpmMode = "enabled"
	expected.TpmModeStatus = 0
	expected.Keyladder = "prod"
	expected.TpmEnabled = true
	expected.ProdKeyladder = true
	expected.FactoryModeValid = false
	expected.InFactoryMode = false
	expected.EKCert = ""

	testSysinfo(t, input, expectedMap, expected)
}

func TestCr50Sysinfo4(t *testing.T) {
	input := `
Reset flags: 0x00000140 (hibernate wake-pin)
Reset count: 1
Chip:        g cr50 B2-C
RO keyid:    0xaa66150f
RW keyid:    0x87b73b67
DEV_ID:      0x11111111 0xaaaaaaaa
Rollback:    2/2/2 4/4/4
TPM MODE:    enabled (0)
Key Ladder:  prod
`
	expectedMap := make(map[string]string)

	expectedMap["breadcrumbs"] = ""
	expectedMap["resetFlags"] = "00000140"
	expectedMap["resetCount"] = "1"
	expectedMap["chipName"] = "cr50"
	expectedMap["chipSKU"] = "B2-C"
	expectedMap["roKeyid"] = "0xaa66150f"
	expectedMap["rwKeyid"] = "0x87b73b67"
	expectedMap["devid"] = "0x11111111 0xaaaaaaaa"
	expectedMap["roRollback"] = "2/2/2"
	expectedMap["rwRollback"] = "4/4/4"
	expectedMap["tpmMode"] = "enabled"
	expectedMap["tpmModeStatus"] = "0"
	expectedMap["keyladder"] = "prod"
	expectedMap["ekCert"] = ""
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.Breadcrumbs = ""
	expected.ResetFlags = GscResetFlagHibernate
	expected.OriginalResetFlags = 0x00000140
	expected.ResetCount = 1
	expected.ChipName = "cr50"
	expected.ChipSKU = SKUH1Clamshell
	expected.ROKeyid = "0xaa66150f"
	expected.RWKeyid = "0x87b73b67"
	expected.Devid = "0x11111111 0xaaaaaaaa"
	expected.RORollback = "2/2/2"
	expected.RWRollback = "4/4/4"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 4, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: 4, Valid: true},
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
	}
	expected.TpmMode = "enabled"
	expected.TpmModeStatus = 0
	expected.Keyladder = "prod"
	expected.TpmEnabled = true
	expected.ProdKeyladder = true
	expected.FactoryModeValid = false
	expected.InFactoryMode = false
	expected.EKCert = ""

	testSysinfo(t, input, expectedMap, expected)
}

func TestCr50Sysinfo5(t *testing.T) {
	input := `
Reset flags: 0x00000800 ()
Reset count: 1
Chip:        g cr50 B2-C
RO keyid:    0xaa66150f(prod)
RW keyid:    0x87b73b67(prod)
DEV_ID:      0x12345678 0x12345678
Rollback:    2/2/2
TPM MODE:    enabled (0)
Key Ladder:  enabled
`
	expectedMap := make(map[string]string)
	expectedMap["breadcrumbs"] = ""
	expectedMap["resetFlags"] = "00000800"
	expectedMap["resetCount"] = "1"
	expectedMap["chipName"] = "cr50"
	expectedMap["chipSKU"] = "B2-C"
	expectedMap["rwKeyid"] = "0x87b73b67"
	expectedMap["roKeyid"] = "0xaa66150f"
	expectedMap["devid"] = "0x12345678 0x12345678"
	expectedMap["roRollback"] = ""
	expectedMap["rwRollback"] = "2/2/2"
	expectedMap["tpmMode"] = "enabled"
	expectedMap["tpmModeStatus"] = "0"
	expectedMap["keyladder"] = "enabled"
	expectedMap["ekCert"] = ""
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = ""

	expected := Sysinfo{}
	expected.Breadcrumbs = ""
	expected.OriginalResetFlags = 0x00000800
	expected.ResetFlags = GscResetFlagHard
	expected.ResetCount = 1
	expected.ChipName = "cr50"
	expected.ChipSKU = SKUH1Clamshell
	expected.RWKeyid = "0x87b73b67"
	expected.ROKeyid = "0xaa66150f"
	expected.Devid = "0x12345678 0x12345678"
	expected.RORollback = ""
	expected.RWRollback = "2/2/2"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 2, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: 2, Valid: true},
		Flash: SysinfoRollbackSlot{Bits: 2, Valid: true},
	}
	expected.TpmMode = "enabled"
	expected.TpmModeStatus = 0
	expected.Keyladder = "enabled"
	expected.TpmEnabled = true
	expected.ProdKeyladder = true
	expected.FactoryModeValid = false
	expected.InFactoryMode = false
	expected.EKCert = ""

	testSysinfo(t, input, expectedMap, expected)
}

func TestCr50Sysinfo6RollbackDetected(t *testing.T) {
	input := `
Reset flags: 0x00000800 ()
Rollback detected
Reset count: 7
Chip:        g cr50 B2-C
RO keyid:    0xaa66150f
RW keyid:    0x334f70df
DEV_ID:      0x12345678 0x12345678
Rollback:    2/2/2 4/128/2
TPM MODE:    enabled (0)
Key Ladder:  dev
`
	expectedMap := make(map[string]string)
	expectedMap["breadcrumbs"] = ""
	expectedMap["resetFlags"] = "00000800"
	expectedMap["resetCount"] = "7"
	expectedMap["chipName"] = "cr50"
	expectedMap["chipSKU"] = "B2-C"
	expectedMap["rwKeyid"] = "0x334f70df"
	expectedMap["roKeyid"] = "0xaa66150f"
	expectedMap["devid"] = "0x12345678 0x12345678"
	expectedMap["roRollback"] = "2/2/2"
	expectedMap["rwRollback"] = "4/128/2"
	expectedMap["tpmMode"] = "enabled"
	expectedMap["tpmModeStatus"] = "0"
	expectedMap["keyladder"] = "dev"
	expectedMap["ekCert"] = ""
	expectedMap["factoryMode"] = ""
	expectedMap["rollbackDetected"] = "Rollback detected"

	expected := Sysinfo{}
	expected.Breadcrumbs = ""
	expected.OriginalResetFlags = 0x00000800
	expected.ResetFlags = GscResetFlagHard
	expected.ResetCount = 7
	expected.ChipName = "cr50"
	expected.ChipSKU = SKUH1Clamshell
	expected.RWKeyid = "0x334f70df"
	expected.ROKeyid = "0xaa66150f"
	expected.Devid = "0x12345678 0x12345678"
	expected.RollbackDetected = true
	expected.RORollback = "2/2/2"
	expected.RWRollback = "4/128/2"
	expected.RWRollbackBits = SysinfoRollbackBits{
		SlotA: SysinfoRollbackSlot{Bits: 128, Valid: true},
		SlotB: SysinfoRollbackSlot{Bits: 2, Valid: true},
		Flash: SysinfoRollbackSlot{Bits: 4, Valid: true},
	}
	expected.TpmMode = "enabled"
	expected.TpmModeStatus = 0
	expected.Keyladder = "dev"
	expected.TpmEnabled = true
	expected.ProdKeyladder = false
	expected.FactoryModeValid = false
	expected.InFactoryMode = false
	expected.EKCert = ""

	testSysinfo(t, input, expectedMap, expected)
}

func testSysinfo(t *testing.T, input string, expectedMap map[string]string, expected Sysinfo) {
	result, err := parseSysinfo(input)
	if err != nil {
		t.Fatal("error processing input:", err)
	}
	if !reflect.DeepEqual(expectedMap, result) {
		t.Fatalf("chip mismatch:\ngot      %+v\nexpected %+v", result, expectedMap)
	}
	sysinfo, err := getSysinfoStruct(result)
	if err != nil {
		t.Fatal("error processing map:", err)
	}
	if !reflect.DeepEqual(expected, sysinfo) {
		t.Fatalf("chip mismatch:\ngot      %+v\nexpected %+v", sysinfo, expected)
	}
}

func TestConvertCr50ResetFlags1(t *testing.T) {
	input := uint32(Cr50ResetFlagPowerOn | Cr50ResetFlagHibernate | Cr50ResetFlagHard | Cr50ResetFlagRdd | Cr50ResetFlagRbox)
	expected := uint32(GscResetFlagPowerOn | GscResetFlagHibernate | GscResetFlagHard | GscResetFlagRdd | GscResetFlagRbox)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags2(t *testing.T) {
	input := uint32(0xffffffff)
	expected := uint32(GscResetFlagPowerOn | GscResetFlagHibernate | GscResetFlagHard | GscResetFlagRdd | GscResetFlagRbox)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags3(t *testing.T) {
	input := uint32(Cr50ResetFlagPowerOn)
	expected := uint32(GscResetFlagPowerOn)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags4(t *testing.T) {
	input := uint32(Cr50ResetFlagHard)
	expected := uint32(GscResetFlagHard)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags5(t *testing.T) {
	input := uint32(Cr50ResetFlagHibernate)
	expected := uint32(GscResetFlagHibernate)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags6(t *testing.T) {
	input := uint32(Cr50ResetFlagRdd)
	expected := uint32(GscResetFlagRdd)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags7(t *testing.T) {
	input := uint32(Cr50ResetFlagRbox)
	expected := uint32(GscResetFlagRbox)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags8(t *testing.T) {
	input := uint32(Cr50ResetFlagHibernate | Cr50ResetFlagRbox)
	expected := uint32(GscResetFlagHibernate | GscResetFlagRbox)
	testConvertCr50ResetFlags(t, input, expected)
}

func TestConvertCr50ResetFlags9(t *testing.T) {
	input := uint32(Cr50ResetFlagHibernate | Cr50ResetFlagRbox)
	expected := uint32(GscResetFlagHibernate | GscResetFlagRbox)
	testConvertCr50ResetFlags(t, input, expected)
}

func testConvertCr50ResetFlags(t *testing.T, input, expected uint32) {
	res := convertCr50ResetFlags(uint64(input))
	if expected != res {
		t.Fatalf("flag mismatch with %x: expected %x got %x", input, expected, res)
	}
}

func TestConvertOTResetFlags1(t *testing.T) {
	input := uint32(OTResetFlagPowerOn | OTResetFlagHibernate | OTResetFlagHard | OTResetBrownOut | OTResetFlagRbox)
	expected := uint32(GscResetFlagPowerOn | GscResetFlagHibernate | GscResetFlagHard | GscResetBrownOut | GscResetFlagRbox)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags2(t *testing.T) {
	input := uint32(0xffffffff)
	expected := uint32(GscResetFlagPowerOn | GscResetFlagHibernate | GscResetFlagHard | GscResetBrownOut | GscResetFlagRbox)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags3(t *testing.T) {
	input := uint32(OTResetFlagPowerOn)
	expected := uint32(GscResetFlagPowerOn)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags4(t *testing.T) {
	input := uint32(OTResetFlagHard)
	expected := uint32(GscResetFlagHard)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags5(t *testing.T) {
	input := uint32(OTResetFlagHibernate)
	expected := uint32(GscResetFlagHibernate)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags6(t *testing.T) {
	input := uint32(OTResetBrownOut)
	expected := uint32(GscResetBrownOut)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags7(t *testing.T) {
	input := uint32(OTResetFlagRbox)
	expected := uint32(GscResetFlagRbox)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags8(t *testing.T) {
	input := uint32(OTResetFlagHibernate | OTResetFlagRbox)
	expected := uint32(GscResetFlagHibernate | GscResetFlagRbox)
	testConvertOTResetFlags(t, input, expected)
}

func TestConvertOTResetFlags9(t *testing.T) {
	input := uint32(OTResetFlagHibernate | OTResetFlagRbox)
	expected := uint32(GscResetFlagHibernate | GscResetFlagRbox)
	testConvertOTResetFlags(t, input, expected)
}

func testConvertOTResetFlags(t *testing.T, input, expected uint32) {
	res := convertOTResetFlags(uint64(input))
	if expected != res {
		t.Fatalf("flag mismatch with %x: expected %x got %x", input, expected, res)
	}
}

func TestChipBID1(t *testing.T) {
	input := `Board ID: ffffffff:ffffffff, flags: ffffffff`
	expected := ChipBID{
		Type:           0xffffffff,
		TypeInv:        0xffffffff,
		Flags:          0xffffffff,
		IsErased:       true,
		FlagsAreErased: true,
		TypeIsErased:   true,
	}
	testparseChipBID(t, input, expected, false)
}

func TestChipBID2(t *testing.T) {
	input := `Board ID: ffffffff:00000000, flags: ffffffff`
	expected := ChipBID{
		Type:           0xffffffff,
		TypeInv:        0,
		Flags:          0xffffffff,
		IsErased:       false,
		FlagsAreErased: false,
		TypeIsErased:   false,
	}
	testparseChipBID(t, input, expected, false)
}

func TestChipBID3(t *testing.T) {
	input := `Board ID: 00000000:ffffffff, flags: ffffffff`
	expected := ChipBID{
		Type:           0,
		TypeInv:        0xffffffff,
		Flags:          0xffffffff,
		IsErased:       false,
		FlagsAreErased: false,
		TypeIsErased:   false,
	}
	testparseChipBID(t, input, expected, false)
}

func TestChipBID4(t *testing.T) {
	input := `Board ID: ffffffff:ffffffff, flags: 00000010`
	expected := ChipBID{
		Type:           0xffffffff,
		TypeInv:        0xffffffff,
		Flags:          0x10,
		IsErased:       false,
		FlagsAreErased: false,
		TypeIsErased:   true,
	}
	testparseChipBID(t, input, expected, false)
}

func TestChipBID5(t *testing.T) {
	input := `Board ID: 4a465a42:b5b9a5bd, flags 00000010`
	expected := ChipBID{
		Type:           0x4a465a42,
		TypeInv:        0xb5b9a5bd,
		Flags:          0x10,
		IsErased:       false,
		FlagsAreErased: false,
		TypeIsErased:   false,
	}
	testparseChipBID(t, input, expected, false)
}

func TestChipBID6(t *testing.T) {
	input := `Board ID: 4A465A42:B5B9A5BD, flags 00000010`
	expected := ChipBID{
		Type:           0x4a465a42,
		TypeInv:        0xb5b9a5bd,
		Flags:          0x10,
		IsErased:       false,
		FlagsAreErased: false,
		TypeIsErased:   false,
	}
	testparseChipBID(t, input, expected, false)
}

func TestChipBID7(t *testing.T) {
	input := `Board ID: gggggggg:gggggggg, flags 00000010`
	testparseChipBID(t, input, ChipBID{}, true)
}

func TestChipBID8(t *testing.T) {
	input := `Board ID: ffffffff:00000000, flags gggggggg`
	testparseChipBID(t, input, ChipBID{}, true)
}
func TestChipBID9(t *testing.T) {
	input := `Board ID: ffffffff:00000001, flags 00000010`
	testparseChipBID(t, input, ChipBID{}, true)
}

func testparseChipBID(t *testing.T, input string, expected ChipBID, expectError bool) {
	chip, err := parseChipBID(input)
	if expectError {
		if err == nil {
			t.Fatalf("%s did not trigger an error", input)
		}
		return
	}
	if err != nil {
		t.Fatal("error processing bid input:", err)
	}
	if expected != chip {
		t.Fatalf("bid mismatch:\ngot      %v\nexpected %v", chip, expected)
	}
}

func TestErasedInfoPages1(t *testing.T) {
	bid := ChipBID{IsErased: true}
	sysinfo := Sysinfo{FactoryModeValid: false, InFactoryMode: false}
	factoryConfig := uint64(0)
	testwriteOnceInfoPagesAreErased(t, bid, sysinfo, factoryConfig, true)
}

func TestErasedInfoPages2(t *testing.T) {
	bid := ChipBID{IsErased: false}
	sysinfo := Sysinfo{FactoryModeValid: false, InFactoryMode: false}
	factoryConfig := uint64(0)
	testwriteOnceInfoPagesAreErased(t, bid, sysinfo, factoryConfig, false)
}

func TestErasedInfoPages3(t *testing.T) {
	bid := ChipBID{IsErased: false}
	sysinfo := Sysinfo{FactoryModeValid: true, InFactoryMode: true}
	factoryConfig := uint64(0)
	testwriteOnceInfoPagesAreErased(t, bid, sysinfo, factoryConfig, false)
}

func TestErasedInfoPages4(t *testing.T) {
	bid := ChipBID{IsErased: true}
	sysinfo := Sysinfo{FactoryModeValid: true, InFactoryMode: false}
	factoryConfig := uint64(0)
	testwriteOnceInfoPagesAreErased(t, bid, sysinfo, factoryConfig, false)
}

func TestErasedInfoPages5(t *testing.T) {
	bid := ChipBID{IsErased: true}
	sysinfo := Sysinfo{FactoryModeValid: true, InFactoryMode: true}
	factoryConfig := uint64(0)
	testwriteOnceInfoPagesAreErased(t, bid, sysinfo, factoryConfig, true)
}

func TestErasedInfoPages6(t *testing.T) {
	bid := ChipBID{IsErased: true}
	sysinfo := Sysinfo{FactoryModeValid: true, InFactoryMode: true}
	factoryConfig := uint64(1)
	testwriteOnceInfoPagesAreErased(t, bid, sysinfo, factoryConfig, false)
}

func testwriteOnceInfoPagesAreErased(t *testing.T, bid ChipBID, sysinfo Sysinfo, factoryConfig uint64, expected bool) {
	got := writeOnceInfoPagesAreErased(bid, sysinfo, factoryConfig)
	if expected != got {
		t.Fatalf("info is erased mismatch:\ngot      %v\nexpected %v\n with bid:%+v sysinfo%+v factoryConfig %x", got, expected, bid, sysinfo, factoryConfig)
	}
}

func TestParseFactoryConfig1(t *testing.T) {
	input := `
brdprop
brdprop
properties = 0x200041
Strap config: LLdd => TPM Bus: SPI; FormFactor: Tablet
Factory config: 0x0
`
	testParseFactoryConfig(t, input, 0, true)
}

func TestParseFactoryConfig2(t *testing.T) {
	input := `
brdprop
brdprop
properties = 0x200041
Strap config: LLdd => TPM Bus: SPI; FormFactor: Tablet
Factory config: 0xA
`
	testParseFactoryConfig(t, input, 10, true)
}

func TestParseFactoryConfig3(t *testing.T) {
	input := `
brdprop
brdprop
properties = 0x200041
Strap config: LLdd => TPM Bus: SPI; FormFactor: Tablet
Factory config: 0xf00000000000000b
`
	testParseFactoryConfig(t, input, 0xf00000000000000b, true)
}

func TestParseFactoryConfig4(t *testing.T) {
	input := `
brdprop
properties = 0x42
tpm board cfg = 0x0
fc = 0xf0000000fa000000
`
	testParseFactoryConfig(t, input, 0xf0000000fa000000, true)
}

func testParseFactoryConfig(t *testing.T, input string, expected uint64, expectError bool) {
	config, err := parseFactoryConfig(input)
	if err != nil {
		t.Fatal("error processing brdprop factory config input:", err)
	}
	if expected != config {
		t.Fatalf("factory config mismatch:\ngot      %v\nexpected %v", config, expected)
	}
}

func TestParseCr50APROInfo1(t *testing.T) {
	input := `
result    : 0
supported : yes (0)
gbbd      : na (10)
sha256 hash b0863a864df1a0f2d4df2393ae4390fed6e1f01ccbdd562969da9b530e8ece2e
Covered ranges:
00c10000...00c107ff
00c11000...00cfffff
`
	expected := Cr50APROInfo{}
	expected.Result = 0
	expected.Supported = true
	expected.SupportedReason = 0
	expected.Hash = "b0863a864df1a0f2d4df2393ae4390fed6e1f01ccbdd562969da9b530e8ece2e"
	testParseCr50APROInfo(t, input, expected, false)
}

func TestParseCr50APROInfo2(t *testing.T) {
	input := `
result    : 4
supported : no (10)
`
	expected := Cr50APROInfo{}
	expected.Result = 4
	expected.Supported = false
	expected.SupportedReason = 10
	expected.Hash = ""
	testParseCr50APROInfo(t, input, expected, false)
}

func TestParseCr50APROInfo3(t *testing.T) {
	input := `
result    : 4
supported : yes (0)
`
	expected := Cr50APROInfo{}
	testParseCr50APROInfo(t, input, expected, true)
}

func testParseCr50APROInfo(t *testing.T, input string, expected Cr50APROInfo, expectError bool) {
	apROInfo, err := parseCr50APROInfo(input)
	if expectError {
		if err == nil {
			t.Fatalf("%s did not trigger an error", input)
		}
		return
	}
	if err != nil {
		t.Fatal("error processing brdprop ap_ro_info input:", err)
	}
	if expected != apROInfo {
		t.Fatalf("ap ro info mismatch:\ngot      %v\nexpected %v", apROInfo, expected)
	}
	if !cmp.Equal(expected, apROInfo) {
		t.Fatalf("chip mismatch:\ngot      %+v\nexpected %+v", apROInfo, expected)
	}
}

func TestParseTi50CCDState1(t *testing.T) {
	input := `
> ccdstate
ccdstate
pcr0:          89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7
AP:            on
Servo:         connected
Rdd:           connected
KeepAlive:     disabled
CCD_MODE:      asserted
State flags:   UARTAP UARTEC UARTFPMCU
CCD Ports blocked: (none)
`
	expected := CCDStateInfo{
		DeepSleepDisabled: CCDStateVal{State: CCDStateNA, Raw: ""},
		AP:                CCDStateVal{State: CCDStateOn, Raw: "on"},
		APUART:            CCDStateVal{State: CCDStateNA, Raw: ""},
		PCR0:              "89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7",
		EC:                CCDStateVal{State: CCDStateNA, Raw: ""},
		Servo:             CCDStateVal{State: CCDStateOn, Raw: "connected"},
		Rdd:               CCDStateVal{State: CCDStateOn, Raw: "connected"},
		KeepAlive:         CCDStateVal{State: CCDStateOff, Raw: "disabled"},
		CCDModeSignal:     CCDStateVal{State: CCDStateOn, Raw: "asserted"},
		StateFlags:        "UARTAP UARTEC UARTFPMCU",
		CCDBlocked:        "(none)",
	}
	testParseCCDStateInfo(t, input, expected, false)
}

func TestParseTi50CCDState2(t *testing.T) {
	input := `
ccdstate
pcr0:          89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7
AP:            on
Servo:         undetectable
Rdd:           connected
KeepAlive:     disabled
CCD_MODE:      asserted
State flags:   UARTAP+TX UARTEC+TX UARTFPMCU+TX
CCD Ports blocked: (none)
`
	expected := CCDStateInfo{
		DeepSleepDisabled: CCDStateVal{State: CCDStateNA, Raw: ""},
		AP:                CCDStateVal{State: CCDStateOn, Raw: "on"},
		APUART:            CCDStateVal{State: CCDStateNA, Raw: ""},
		PCR0:              "89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7",
		EC:                CCDStateVal{State: CCDStateNA, Raw: ""},
		Servo:             CCDStateVal{State: CCDStateUndetectable, Raw: "undetectable"},
		Rdd:               CCDStateVal{State: CCDStateOn, Raw: "connected"},
		KeepAlive:         CCDStateVal{State: CCDStateOff, Raw: "disabled"},
		CCDModeSignal:     CCDStateVal{State: CCDStateOn, Raw: "asserted"},
		StateFlags:        "UARTAP+TX UARTEC+TX UARTFPMCU+TX",
		CCDBlocked:        "(none)",
	}
	testParseCCDStateInfo(t, input, expected, false)
}

func TestParseTi50CCDState3(t *testing.T) {
	input := `
ccdstate
pcr0:          0000000000000000000000000000000000000000000000000000000000000000
AP:            on
Servo:         connected
Rdd:           disconnected
KeepAlive:     disabled
CCD_MODE:      deasserted
State flags:
CCD ports blocked: (none)
`
	expected := CCDStateInfo{
		DeepSleepDisabled: CCDStateVal{State: CCDStateNA, Raw: ""},
		AP:                CCDStateVal{State: CCDStateOn, Raw: "on"},
		APUART:            CCDStateVal{State: CCDStateNA, Raw: ""},
		PCR0:              "0000000000000000000000000000000000000000000000000000000000000000",
		EC:                CCDStateVal{State: CCDStateNA, Raw: ""},
		Servo:             CCDStateVal{State: CCDStateOn, Raw: "connected"},
		Rdd:               CCDStateVal{State: CCDStateOff, Raw: "disconnected"},
		KeepAlive:         CCDStateVal{State: CCDStateOff, Raw: "disabled"},
		CCDModeSignal:     CCDStateVal{State: CCDStateOff, Raw: "deasserted"},
		StateFlags:        "",
		CCDBlocked:        "(none)",
	}
	testParseCCDStateInfo(t, input, expected, false)
}

func TestParseTi50CCDState4(t *testing.T) {
	input := `
pcr0:          0000000000000000000000000000000000000000000000000000000000000000
AP:            on
Servo:         debouncing
Rdd:           disconnected
KeepAlive:     disabled
CCD_MODE:      deasserted
State flags:
CCD ports blocked: (none)
`
	expected := CCDStateInfo{
		DeepSleepDisabled: CCDStateVal{State: CCDStateNA, Raw: ""},
		AP:                CCDStateVal{State: CCDStateOn, Raw: "on"},
		APUART:            CCDStateVal{State: CCDStateNA, Raw: ""},
		PCR0:              "0000000000000000000000000000000000000000000000000000000000000000",
		EC:                CCDStateVal{State: CCDStateNA, Raw: ""},
		Servo:             CCDStateVal{State: CCDStateDebouncing, Raw: "debouncing"},
		Rdd:               CCDStateVal{State: CCDStateOff, Raw: "disconnected"},
		KeepAlive:         CCDStateVal{State: CCDStateOff, Raw: "disabled"},
		CCDModeSignal:     CCDStateVal{State: CCDStateOff, Raw: "deasserted"},
		StateFlags:        "",
		CCDBlocked:        "(none)",
		IsDebouncing:      true,
	}
	testParseCCDStateInfo(t, input, expected, false)
}

func TestParseCr50CCDState1(t *testing.T) {
	input := `
DS Dis:  off
AP:      on (K)
AP UART: on
pcr0:    89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7
EC:      on
Servo:   connected
Rdd:       connected
KeepAlive: enabled
CCD_MODE:  asserted
State flags: UARTAP UARTEC USBEC+TX
CCD ports blocked: (none)
`
	expected := CCDStateInfo{
		DeepSleepDisabled: CCDStateVal{State: CCDStateOff, Raw: "off"},
		AP:                CCDStateVal{State: CCDStateOn, Raw: "on (K)"},
		APUART:            CCDStateVal{State: CCDStateOn, Raw: "on"},
		PCR0:              "89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7",
		EC:                CCDStateVal{State: CCDStateOn, Raw: "on"},
		Servo:             CCDStateVal{State: CCDStateOn, Raw: "connected"},
		Rdd:               CCDStateVal{State: CCDStateOn, Raw: "connected"},
		KeepAlive:         CCDStateVal{State: CCDStateOn, Raw: "enabled"},
		CCDModeSignal:     CCDStateVal{State: CCDStateOn, Raw: "asserted"},
		StateFlags:        "UARTAP UARTEC USBEC+TX",
		CCDBlocked:        "(none)",
	}
	testParseCCDStateInfo(t, input, expected, false)
}

func TestParseCr50CCDState2(t *testing.T) {
	input := `
> ccdstate
DS Dis:  off
AP:      on (K)
AP UART: on
pcr0:    89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7
EC:      on
Servo:   undetectable
Rdd:       connected
KeepAlive: disabled
CCD_MODE:  asserted
State flags: UARTAP+TX UARTEC+TX I2C SPI USBEC+TX
CCD ports blocked: (none)
`
	expected := CCDStateInfo{
		DeepSleepDisabled: CCDStateVal{State: CCDStateOff, Raw: "off"},
		AP:                CCDStateVal{State: CCDStateOn, Raw: "on (K)"},
		APUART:            CCDStateVal{State: CCDStateOn, Raw: "on"},
		PCR0:              "89eaf35134b4b3c649f44c0c765b96aeab8bb34ee83cc7a683c4e53d1581c8c7",
		EC:                CCDStateVal{State: CCDStateOn, Raw: "on"},
		Servo:             CCDStateVal{State: CCDStateUndetectable, Raw: "undetectable"},
		Rdd:               CCDStateVal{State: CCDStateOn, Raw: "connected"},
		KeepAlive:         CCDStateVal{State: CCDStateOff, Raw: "disabled"},
		CCDModeSignal:     CCDStateVal{State: CCDStateOn, Raw: "asserted"},
		StateFlags:        "UARTAP+TX UARTEC+TX I2C SPI USBEC+TX",
		CCDBlocked:        "(none)",
	}
	testParseCCDStateInfo(t, input, expected, false)
}

func testParseCCDStateInfo(t *testing.T, input string, expected CCDStateInfo, expectError bool) {
	ccdState, err := parseCCDStateInfo(input)
	if expectError {
		if err == nil {
			t.Fatalf("%s did not trigger an error", input)
		}
		return
	}
	if err != nil {
		t.Fatal("error processing brdprop ccdstate input:", err)
	}

	if expected != ccdState {
		t.Fatalf("ccdstate mismatch:\ngot %+v\nexpected %+v", ccdState, expected)
	}
	if !cmp.Equal(expected, ccdState) {
		t.Fatalf("ccdState mismatch:\ngot %+v\nexpected %+v", ccdState, expected)
	}
}

func TestParseBoardConfig1(t *testing.T) {
	input := `
properties = 0x242
tpm board cfg = 0x0
fc = 0x0000000000000000
board_cfg = 0x00000000
`
	testParseBoardConfig(t, input, 0)
}

func TestParseBoardConfig2(t *testing.T) {
	input := `
properties = 0x242
tpm board cfg = 0x0
fc = 0x0000000000000000
board_cfg = 0x00020000
`
	testParseBoardConfig(t, input, 0x20000)
}

func testParseBoardConfig(t *testing.T, input string, expected uint32) {
	boardConfig, err := parseBoardConfig(input)
	if err != nil {
		t.Fatal("error processing brdprop board config input:", err)
	}
	if expected != boardConfig {
		t.Fatalf("board config mismatch:\ngot      %v\nexpected %v", boardConfig, expected)
	}
}

func TestParseStrongboxState1(t *testing.T) {
	testParseStrongboxState(t, 0, StrongboxUnset, false)
}

func TestParseStrongboxState2(t *testing.T) {
	testParseStrongboxState(t, 0xfffcffff, StrongboxUnset, false)
}

func TestParseStrongboxState3(t *testing.T) {
	testParseStrongboxState(t, 0x20000, StrongboxDisabled, false)
}

func TestParseStrongboxState4(t *testing.T) {
	testParseStrongboxState(t, 0x10000, StrongboxEnabled, false)
}

func TestParseStrongboxState5(t *testing.T) {
	testParseStrongboxState(t, 0x30000, StrongboxUnset, true)
}

func testParseStrongboxState(t *testing.T, input uint32, expected StrongboxState, expectError bool) {
	state, err := parseStrongboxState(input)
	if expectError {
		if err == nil {
			t.Fatalf("%x did not generate a strongbox state error", input)
		}
		return
	}
	if err != nil {
		t.Fatal("error processing strongbox state:", err)
	}
	if expected != state {
		t.Fatalf("strongbox state mismatch:\ngot      %v\nexpected %v", state, expected)
	}
}
