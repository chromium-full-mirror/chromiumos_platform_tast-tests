// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"encoding/hex"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
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

	response, err := t.Execute(tpmvGetBootMode)
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

	_, err = t.Execute(tpmvCommitNvmem)
	return err
}
