// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pkcs11

import (
	"crypto/x509"
	"testing"

	"go.chromium.org/tast/core/errors"
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

func fakeCertReaderSuccess(ckaID string) (*x509.Certificate, error) {
	return nil, nil
}

func fakeCertReaderFailure(ckaID string) (*x509.Certificate, error) {
	return nil, errors.New("failure")
}

func TestParseCertsOk(t *testing.T) {
	input := `
Private Key Object; RSA
  label:      Device_5CD2501TVS's example-NEW-CERT-PROV-T-CA ID
  ID:         76784a14bb8fcecd68c2e2d426270d40326c6b81
  Usage:      sign
  Access:     sensitive
Certificate Object; type = X.509 cert
  label:      Device_5CD2501TVS's example-NEW-CERT-PROV-T-CA ID
  subject:    DN: O=TestCompanyNameForDevice, OU=TestCompanyNameForDevice, CN=Device_5CD2501TVS
  ID:         76784a14bb8fcecd68c2e2d426270d40326c6b81
Certificate Object; type = X.509 cert
  label:      SecondCert label
  subject:    DN: CN=SecondCert
  ID:         399672eaa648114f4bfc18d0bb52510a7a0ca3cd`

	sections, err := parsePkcs11ToolOutput(input)
	if err != nil {
		t.Fatal("Unexpected failure: ", err)
	}

	certs, err := parseAndReadCerts(sections, fakeCertReaderSuccess)

	if err != nil {
		t.Fatal("Unexpected fail: ", err)
	}
	if len(certs) != 2 {
		t.Fatalf("Expected 2 certs, got %d", len(certs))
	}

	if certs[0].ckaID != "76784a14bb8fcecd68c2e2d426270d40326c6b81" {
		t.Errorf("Wrong ckaID in certs[0] - expected \"76784a14bb8fcecd68c2e2d426270d40326c6b81\", got \"%s\"", certs[0].ckaID)
	}
	if certs[1].ckaID != "399672eaa648114f4bfc18d0bb52510a7a0ca3cd" {
		t.Errorf("Wrong ckaID in certs[1] - expected \"399672eaa648114f4bfc18d0bb52510a7a0ca3cd\", got \"%s\"", certs[1].ckaID)
	}
}

func TestParseCertsNoID(t *testing.T) {
	sections := []section{
		{
			header: "Certificate Object",
			keys: map[string]string{
				"unrelated key": "Token 1",
			},
		},
	}

	if _, err := parseAndReadCerts(sections, fakeCertReaderSuccess); err == nil {
		t.Error("Expected error from parseAndReadCerts")
	}
}

func TestParseCertsEmptyID(t *testing.T) {
	sections := []section{
		{
			header: "Certificate Object",
			keys: map[string]string{
				"ID": "",
			},
		},
	}

	if _, err := parseAndReadCerts(sections, fakeCertReaderSuccess); err == nil {
		t.Error("Expected error from parseAndReadCerts")
	}
}

func TestParseCertsCertReaderFailure(t *testing.T) {
	sections := []section{
		{
			header: "Certificate Object",
			keys: map[string]string{
				"ID": "Token 1",
			},
		},
	}

	if _, err := parseAndReadCerts(sections, fakeCertReaderFailure); err == nil {
		t.Error("Expected error from parseAndReadCerts")
	}
}
