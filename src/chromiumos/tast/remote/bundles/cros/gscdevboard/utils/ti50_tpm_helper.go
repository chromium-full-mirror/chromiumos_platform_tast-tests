// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/hex"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpmutil"

	"chromiumos/tast/common/firmware/ti50"
)

const (
	// EmptyPassword is blank password used for authentication
	EmptyPassword = ""
	// RootPlatformHandle is the Platform Root TPM Handle
	RootPlatformHandle tpmutil.Handle = 0x4000000c
	// KernelFileID is the NVMem ID for the kernel file
	KernelFileID tpmutil.Handle = 0x01001008
	// KernelFileAttr is the attribute set that AP firmware uses when creating kernel file
	KernelFileAttr = tpm2.AttrPlatformCreate |
		tpm2.AttrAuthRead |
		tpm2.AttrPPRead |
		tpm2.AttrWriteSTClear |
		tpm2.AttrPPWrite
)

// TpmHandle allows interacting with GSC's TPM bus with higher level tpm commands until tpm2 lib
type TpmHandle struct {
	h        DevboardHelper
	ctx      context.Context
	bus      ti50.TpmBus
	response []byte // Contains the response to the last issued request.
}

// Write will be called by the go-tpm library to send a command to the TPM.
func (t *TpmHandle) Write(data []byte) (int, error) {
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
func (t *TpmHandle) Read(data []byte) (int, error) {
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

// ReadRegister retrieves the value of a TPM register by communicating via SPI or I2C.
func (t *TpmHandle) ReadRegister(register ti50.TpmRegister) string {
	data, err := t.h.OpenTitanToolCommand(t.ctx,
		string(t.bus), "tpm", "read-register", string(register))
	if err != nil {
		t.h.Fatalf("failed to read TPM register %s: %s", register, err)
	}
	return data["hexdata"].(string)
}

// Execute sends a TPM request using possibly multiple writes to the FIFO and status
// registers, and waits for the execution to complete before retrieving the reply. Only use this
// if the Tpm interface does not provided access, e.g. VendorCommands
func (t *TpmHandle) Execute(request []byte) []byte {
	response, err := t.h.OpenTitanToolCommand(t.ctx,
		string(t.bus), "tpm", "execute-command", "--hexdata", hex.EncodeToString(request))
	if err != nil {
		t.h.Fatalf("failed to execute TPM command: %s", err)
	}
	out, err := hex.DecodeString(response["hexdata"].(string))
	if err != nil {
		t.h.Fatalf("response was not hex: %s", response["hexdata"].(string))
	}
	return out
}
