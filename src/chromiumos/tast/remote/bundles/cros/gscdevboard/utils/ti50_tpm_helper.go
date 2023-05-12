// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"encoding/hex"

	"chromiumos/tast/common/firmware/ti50"
)

// TpmHelper allows interacting with GSC's TPM bus with higher level tpm commands until tpm2 lib
type TpmHelper struct {
	*ti50.TpmHandle
	h DevboardHelper
}

// ReadRegister retrieves the value of a TPM register by communicating via SPI or I2C.
func (t *TpmHelper) ReadRegister(register ti50.TpmRegister) string {
	data, err := t.h.OpenTitanToolCommand(t.Ctx,
		string(t.Bus), "tpm", "read-register", string(register))
	if err != nil {
		t.h.Fatalf("failed to read TPM register %s: %s", register, err)
	}
	return data["hexdata"].(string)
}

// Execute sends a TPM request using possibly multiple writes to the FIFO and status
// registers, and waits for the execution to complete before retrieving the reply. Only use this
// if the Tpm interface does not provided access, e.g. VendorCommands
func (t *TpmHelper) Execute(request []byte) []byte {
	response, err := t.h.OpenTitanToolCommand(t.Ctx,
		string(t.Bus), "tpm", "execute-command", "--hexdata", hex.EncodeToString(request))
	if err != nil {
		t.h.Fatalf("failed to execute TPM command: %s", err)
	}
	out, err := hex.DecodeString(response["hexdata"].(string))
	if err != nil {
		t.h.Fatalf("response was not hex: %s", response["hexdata"].(string))
	}
	return out
}
