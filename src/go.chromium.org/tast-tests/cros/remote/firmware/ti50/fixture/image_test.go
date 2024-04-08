// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"testing"
)

func TestGetBIDInt1(t *testing.T) {
	testGetBIDInt(t, "0x10", 16, false)
}

func TestGetBIDInt2(t *testing.T) {
	testGetBIDInt(t, "10", 16, false)
}

func TestGetBIDInt3(t *testing.T) {
	testGetBIDInt(t, "00000010", 16, false)
}

func TestGetBIDInt4(t *testing.T) {
	testGetBIDInt(t, "00020000", 131072, false)
}

func TestGetBIDInt5(t *testing.T) {
	testGetBIDInt(t, "f", 15, false)
}

func TestGetBIDInt6(t *testing.T) {
	testGetBIDInt(t, "F", 15, false)
}

func TestGetBIDInt7(t *testing.T) {
	testGetBIDInt(t, "0xffffffff", 4294967295, false)
}

func TestGetBIDInt8(t *testing.T) {
	testGetBIDInt(t, "0xfffffffff", 0, true)
}

func TestGetBIDInt9(t *testing.T) {
	testGetBIDInt(t, "GGG", 0, true)
}

func testGetBIDInt(t *testing.T, input string, expected int64, expectError bool) {
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
	testQualVersionToGsGlob(t, "invalidName", "0.3.22", "", true)
}

func TestQualVersionToGsGlob8(t *testing.T) {
	testQualVersionToGsGlob(t, "ti50", "0.24.81/::", "", true)
}

func TestQualVersionToGsGlob9(t *testing.T) {
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
