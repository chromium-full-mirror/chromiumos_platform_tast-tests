// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"testing"
)

func TestBIDInt1(t *testing.T) {
	testBIDInt(t, "0x10", 16, false)
}

func TestBIDInt2(t *testing.T) {
	testBIDInt(t, "10", 16, false)
}

func TestBIDInt3(t *testing.T) {
	testBIDInt(t, "00000010", 16, false)
}

func TestBIDInt4(t *testing.T) {
	testBIDInt(t, "00020000", 131072, false)
}

func TestBIDInt5(t *testing.T) {
	testBIDInt(t, "f", 15, false)
}

func TestBIDInt6(t *testing.T) {
	testBIDInt(t, "F", 15, false)
}

func TestBIDInt7(t *testing.T) {
	testBIDInt(t, "0xffffffff", 4294967295, false)
}

func TestBIDInt8(t *testing.T) {
	testBIDInt(t, "0xfffffffff", 0, true)
}

func TestBIDInt9(t *testing.T) {
	testBIDInt(t, "GGG", 0, true)
}

func testBIDInt(t *testing.T, input string, expected int64, expectError bool) {
	out, err := getBIDInt(input)
	if expectError {
		if err == nil {
			t.Fatalf("Did not get error processing %s", input)
		}
		return
	}
	if err != nil {
		t.Fatal("error processing BID field:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestQualVersionToGsGlob1(t *testing.T) {
	testQualVersionToGsGlob(t, "cr50", "0.3.22", "cr50.r0.0.*.w0.3.22.tbz2", false)
}

func TestQualVersionToGsGlob2(t *testing.T) {
	testQualVersionToGsGlob(t, "cr50", "0.6.241/FFFF:0:0x10", "cr50.r0.0.*.w0.6.241_FFFF_00000000_00000010.tbz2", false)
}

func TestQualVersionToGsGlob3(t *testing.T) {
	testQualVersionToGsGlob(t, "cr50", "0.6.241/46464646:0:0x10", "cr50.r0.0.*.w0.6.241_46464646_00000000_00000010.tbz2", false)
}

func TestQualVersionToGsGlob4(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.23.81", "ti50.r*w*0.23.81.tar.xz", false)
}

func TestQualVersionToGsGlob5(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.23.81/", "ti50.r*w*0.23.81.tar.xz", false)
}

func TestQualVersionToGsGlob6(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.24.81/FFFF:0x00000010:0x10", "ti50.r*w*0.24.81_FFFF_00000010_00000010.tar.xz", false)
}

func TestQualVersionToGsGlob7(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.33.81/", "ti50-nt.r*w*0.33.81.tar.xz", false)
}

func TestQualVersionToGsGlob8(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.34.81/FFFF:0x00000010:0x10", "ti50-nt.r*w*0.34.81_FFFF_00000010_00000010.tar.xz", false)
}

func TestQualVersionToGsGlob9(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.33.81/FFFF:0:0x10000", "ti50-nt.r*w*0.33.81_FFFF_00000000_00010000.tar.xz", false)
}

func TestQualVersionToGsGlob10(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.33.81/FFFF:0:0x100000", "ti50a-nt.r*w*0.33.81_FFFF_00000000_00100000.tar.xz", false)
}

func TestQualVersionToGsGlob11(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.34.81/FFFF:0:0x100010", "ti50a-nt.r*w*0.34.81_FFFF_00000000_00100010.tar.xz", false)
}

func TestQualVersionToGsGlobError1(t *testing.T) {
	testQualVersionToGsGlob(t, "invalidName", "0.3.22", "", true)
}

func TestQualVersionToGsGlobError2(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.24.81/::", "", true)
}

func TestQualVersionToGsGlobError3(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.24.81/FFFF:0:0xZZZZ", "", true)
}

func testQualVersionToGsGlob(t *testing.T, fwName, input, expected string, expectError bool) {
	out, err := qualVersionToGsGlob(input, fwName)
	if expectError {
		if err == nil {
			t.Fatalf("Did not get error processing %s", input)
		}
		return
	}
	if err != nil {
		t.Fatal("error processing BID field:", err)
	}
	if out != expected {
		t.Fatalf("output mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}

func TestQualFwNameExt1(t *testing.T) {
	testQualFwNameExt(t, "0.3.81", 0x10000, "")
}

func TestQualFwNameExt2(t *testing.T) {
	testQualFwNameExt(t, "0.3.81", 0x110000, "a")
}

func TestQualFwNameExt3(t *testing.T) {
	testQualFwNameExt(t, "0.24.81", 0x10000, "")
}

func TestQualFwNameExt4(t *testing.T) {
	testQualFwNameExt(t, "0.24.81", 0x110000, "a")
}

func TestQualFwNameExt5(t *testing.T) {
	testQualFwNameExt(t, "0.34.81", 0x10000, "-nt")
}

func TestQualFwNameExt6(t *testing.T) {
	testQualFwNameExt(t, "0.34.81", 0x110000, "a-nt")
}

func testQualFwNameExt(t *testing.T, rwVersion string, bidFlags int64, expected string) {
	out := qualFwNameExt(rwVersion, bidFlags)
	if out != expected {
		t.Fatalf("chipExt mismatch:\ngot      %v\nexpected %v", out, expected)
	}
}
