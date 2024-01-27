// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"testing"
)

func TestVersionCommandProcessor1(t *testing.T) {
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

func TestVersionCommandProcessor2(t *testing.T) {
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

func TestVersionCommandProcessor3(t *testing.T) {
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
		RwA:   RwInfo{Empty: false, Active: false, Version: "0.6.211", Branch: Unknown},
		RwB:   RwInfo{Empty: false, Active: true, Debug: true, Version: "1.6.205", Branch: Unknown},
		Bid:   BidInfo{Empty: false, BidType: 0, Mask: 0, Flags: 0},
		Build: BuildInfo{Branch: Unknown, Debug: true},
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

func testBuildInfoMatcher(t *testing.T, input string, expected BuildInfo) {
	out, err := matchBuildInfo(input)
	if err != nil {
		t.Fatal("error processing bid info:", err)
	}
	if out != expected {
		t.Fatal("output mismatch")
	}
}
