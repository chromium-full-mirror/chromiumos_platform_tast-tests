// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pkcs11

import (
	"testing"
)

func TestParseSlotsOk(t *testing.T) {
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
	sections, err := parsePkcs11ToolOutput(input)
	if err != nil {
		t.Fatal("Unexpected failure: ", err)
	}

	slots, err := parseSlots(sections)

	if err != nil {
		t.Fatal("Unexpected failure: ", err)
	}
	if len(slots) != 2 {
		t.Fatalf("Expected 2 slots, got %d", len(slots))
	}

	if slots[0].slotIndex != 0 {
		t.Errorf("Wrong slotIndex in slot[0] - expected 0, got %d", slots[0].slotIndex)
	}
	if slots[0].tokenLabel != "System TPM Token" {
		t.Errorf("Wrong tokenLabel in slot[0] - expected \"System TPM Token\", got \"%s\"", slots[0].tokenLabel)
	}
	if slots[1].slotIndex != 1 {
		t.Errorf("Wrong slotIndex in slot[1] - expected 100, got %d", slots[1].slotIndex)
	}
	if slots[1].tokenLabel != "User TPM Token a77c67a54146789a" {
		t.Errorf("Wrong tokenLabel in slot[1] - expected \"User TPM Token a77c67a54146789a\", got \"%s\"", slots[1].tokenLabel)
	}
}

func TestParseSlotsBadIndex(t *testing.T) {
	sections := []section{
		{
			header: "Slot bla",
			keys: map[string]string{
				"token label": "Token 1",
			},
		},
	}

	if _, err := parseSlots(sections); err == nil {
		t.Error("Expected error from parseSlots")
	}
}

func TestParseSlotsNoTokenLabel(t *testing.T) {
	sections := []section{
		{
			header: "Slot 1",
			keys: map[string]string{
				"unrelated key": "Token 1",
			},
		},
	}

	if _, err := parseSlots(sections); err == nil {
		t.Error("Expected error from parseSlots")
	}
}
