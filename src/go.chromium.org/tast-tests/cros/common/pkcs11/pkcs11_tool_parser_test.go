// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pkcs11

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/maps"
)

// TestEmpty checks that an empty imput maps to an empty list of sections and
// does not generate an error.
func TestEmpty(t *testing.T) {
	sections, err := parsePkcs11ToolOutput("")
	if err != nil {
		t.Fatal("Unexpected fail: ", err)
	}
	if len(sections) != 0 {
		t.Errorf("Expected 0 sections, got %d", len(sections))
	}
}

func TestGoodInput(t *testing.T) {
	// Output of
	// pkcs11-tool --module=/usr/lib64/libchaps.so --list-slots
	// On Chrome OS.
	input := `

Available slots:
Slot 0 (0x0): TPM Slot
  token label        : System TPM Token
  token manufacturer : Chromium OS
  token model        :
  token flags        : PIN pad present, rng, token initialized, PIN initialized
  hardware version   : 1.0
  firmware version   : 1.0
  serial num         : Not Available
  pin min/max        : 6/127
Slot 1 (0x1): TPM Slot
  token label        : User TPM Token a77c67a54146789a
  token manufacturer : Chromium OS
  token model        :
  token flags        : PIN pad present, rng, token initialized, PIN initialized
  hardware version   : 1.0
  firmware version   : 1.0
  serial num         : Not Available
  pin min/max        : 6/127
`
	expectedSection0Header := "Available slots:"
	expectedSection1Header := "Slot 0 (0x0): TPM Slot"
	expectedSection2Header := "Slot 1 (0x1): TPM Slot"

	commonKeys := map[string]string{
		"token manufacturer": "Chromium OS",
		"token model":        "",
		"token flags":        "PIN pad present, rng, token initialized, PIN initialized",
		"hardware version":   "1.0",
		"firmware version":   "1.0",
		"serial num":         "Not Available",
		"pin min/max":        "6/127",
	}
	expectedSection1Keys := maps.Clone(commonKeys)
	expectedSection1Keys["token label"] = "System TPM Token"
	expectedSection2Keys := maps.Clone(commonKeys)
	expectedSection2Keys["token label"] = "User TPM Token a77c67a54146789a"

	sections, err := parsePkcs11ToolOutput(input)
	if err != nil {
		t.Fatal("Unexpected fail: ", err)
	}
	if len(sections) != 3 {
		t.Fatalf("Expected 3 sections, got %d", len(sections))
	}

	if sections[0].header != expectedSection0Header {
		t.Errorf("Wrong header in sections[0] - expected \"%s\", got \"%s\"", expectedSection0Header, sections[0].header)
	}
	if len(sections[0].keys) != 0 {
		t.Errorf("Expected no keys in sections[0], got %d", len(sections[0].keys))
	}

	if sections[1].header != expectedSection1Header {
		t.Errorf("Wrong header in sections[1] - expected \"%s\", got \"%s\"", expectedSection1Header, sections[1].header)
	}
	if diff := cmp.Diff(expectedSection1Keys, sections[1].keys); diff != "" {
		t.Errorf("sections[1].keys mismatch (-want +got):\n%s", diff)
	}

	if sections[2].header != expectedSection2Header {
		t.Errorf("Wrong header in sections[2] - expected \"%s\", got \"%s\"", expectedSection2Header, sections[2].header)
	}
	if diff := cmp.Diff(expectedSection2Keys, sections[2].keys); diff != "" {
		t.Errorf("sections[2].keys mismatch (-want +got):\n%s", diff)
	}
}

func TestIgnoreEmptyLines(t *testing.T) {
	input := "   " + `
Section 0

     Key: Value
`
	sections, err := parsePkcs11ToolOutput(input)
	if err != nil {
		t.Fatal("Unexpected fail: ", err)
	}
	if len(sections) != 1 {
		t.Fatalf("Expected 1 section, got %d", len(sections))
	}

	if sections[0].header != "Section 0" {
		t.Errorf("Wrong header in first section - expected \"Section 0\", got \"%s\"", sections[0].header)
	}
	expectedSection0 := map[string]string{
		"Key": "Value",
	}
	if diff := cmp.Diff(expectedSection0, sections[0].keys); diff != "" {
		t.Errorf("sections[0].keys mismatch (-want +got):\n%s", diff)
	}
}

func TestNoSection(t *testing.T) {
	input := `   non-header stuff`
	if _, err := parsePkcs11ToolOutput(input); err == nil {
		t.Error("Expected error from parsePkcs11ToolOutput")
	}
}

func TestMissingColon(t *testing.T) {
	input := `Section
    ColonIsMissing`
	if _, err := parsePkcs11ToolOutput(input); err == nil {
		t.Error("Expected error from parsePkcs11ToolOutput")
	}
}
