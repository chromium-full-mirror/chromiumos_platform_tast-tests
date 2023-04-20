// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/hex"

	"chromiumos/tast/common/firmware/ti50"
)

type tpmHandle struct {
	h        DevboardHelper
	ctx      context.Context
	bus      ti50.TpmBus
	response []byte // Contains the response to the last issued request.
}

// Write will be called by the go-tpm library to send a command to the TPM.
func (t *tpmHandle) Write(data []byte) (int, error) {
	response, err := t.h.OpenTitanToolCommand(t.ctx,
		string(t.bus), "tpm", "execute-command", "--hexdata", string(hex.EncodeToString(data)))
	if err != nil {
		return 0, err
	}
	t.response, err = hex.DecodeString(response["hexdata"].(string))
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// Read will be called by the go-tpm library to retrieve the response of a prior a command.
func (t *tpmHandle) Read(data []byte) (int, error) {
	if t.response == nil {
		return 0, nil
	}
	var respLen int
	if len(data) < len(t.response) {
		respLen = len(data)
	} else {
		respLen = len(t.response)
	}
	copy(data[:respLen], t.response[:respLen])
	t.response = nil
	return respLen, nil
}

// TpmReadRegister retrieves the value of a TPM register by communicating via SPI or I2C.
func (t *tpmHandle) ReadRegister(register ti50.TpmRegister) string {
	data, err := t.h.OpenTitanToolCommand(t.ctx,
		string(t.bus), "tpm", "read-register", string(register))
	if err != nil {
		t.h.Fatalf("failed to read TPM register %s: %s", register, err)
	}
	return data["hexdata"].(string)
}
