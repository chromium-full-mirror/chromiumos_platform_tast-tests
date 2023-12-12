// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"encoding/hex"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
)

const (
	// FWMPDisableDevMode flag disables developer mode
	FWMPDisableDevMode = 1 << 0
	// FWMPDisableUnlock flag disables ccd unlock and other functionality on GSC
	FWMPDisableUnlock = 1 << 6
)

// TpmHelper allows interacting with GSC's TPM bus with higher level tpm commands until tpm2 lib
type TpmHelper struct {
	*ti50.TpmHandle
	h DevboardHelper
}

// ReadRegister retrieves the value of a TPM register by communicating via SPI or I2C.
func (t *TpmHelper) ReadRegister(register ti50.TpmRegister) []byte {
	response, err := t.OpenTitanToolTpmCommand("read-register", string(register))
	if err != nil {
		t.h.Fatalf("failed to read TPM register %s: %s", register, err)
	}
	return response
}

// TpmvGetBootMode reads boot mode via TPM GetBootMode vendor command.
func (t *TpmHelper) TpmvGetBootMode() (byte, error) {
	var tpmvGetBootMode, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0034") // subcommand: GetBootMode

	response, err := t.Send(tpmvGetBootMode)
	if err != nil {
		return 0, err
	}
	mode := response[12]
	return mode, nil
}

// TpmvCommitNvmem sned the CommitNvmem vendor command.
func (t *TpmHelper) TpmvCommitNvmem() error {
	tpmvCommitNvmem, err := hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0015") // subcommand: CommitNvmem
	if err != nil {
		return err
	}

	_, err = t.Send(tpmvCommitNvmem)
	return err
}

// MakeFWMPFile creates the 40 bytes FWMP file with the specified flags
func MakeFWMPFile(flags uint32) []byte {
	out := make([]byte, 40)
	out[0] = 0                 // crc -- filled in later
	out[1] = 40                // size
	out[2] = 0x10              // version 1.0
	out[3] = 0x00              // reserved
	out[4] = byte(flags >> 0)  // flags (1 of 4)
	out[5] = byte(flags >> 8)  // flags (2 of 4)
	out[6] = byte(flags >> 16) // flags (3 of 4)
	out[7] = byte(flags >> 24) // flags (4 of 4)
	out[0] = Crc8(out[2:])     // crc
	return out
}
