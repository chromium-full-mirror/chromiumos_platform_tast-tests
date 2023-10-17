// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"encoding/hex"

	"github.com/google/go-tpm/legacy/tpm2"
	"github.com/google/go-tpm/tpmutil"
)

// TpmRegister represents the name of a TPM register
type TpmRegister string

// Constants representing TPM registers, for use with `OpenTitanToolCommand()`.
const (
	// TpmRegAccess is a TPM register
	TpmRegAccess TpmRegister = "ACCESS"

	// TpmRegIntEnable is a TPM register
	TpmRegIntEnable TpmRegister = "INT_ENABLE"

	// TpmRegIntVector is a TPM register
	TpmRegIntVector TpmRegister = "INT_VECTOR"

	// TpmRegIntStatus is a TPM register
	TpmRegIntStatus TpmRegister = "INT_STATUS"

	// TpmRegIntfCapability is a TPM register
	TpmRegIntfCapability TpmRegister = "INTF_CAPABILITY"

	// TpmRegSts is a TPM register
	TpmRegSts TpmRegister = "STS"

	// TpmRegDataFifo is a TPM register
	TpmRegDataFifo TpmRegister = "DATA_FIFO"

	// TpmRegInterfaceID is a TPM register
	TpmRegInterfaceID TpmRegister = "INTERFACE_ID"

	// TpmRegXdataFifo is a TPM register
	TpmRegXdataFifo TpmRegister = "XDATA_FIFO"

	// TpmRegDidVid is a TPM register
	TpmRegDidVid TpmRegister = "DID_VID"

	// TpmRegRid is a TPM register
	TpmRegRid TpmRegister = "RID"
)

// TpmBus represents the physical means to communicate with the TPM, i.e. SPI or I2C.
type TpmBus string

const (
	// TpmBusSpi means that the TPM is to be reached via SPI
	TpmBusSpi TpmBus = "spi"

	// TpmBusI2c means that the TPM is to be reached via I2C
	TpmBusI2c TpmBus = "i2c"
)

// TpmI2cAddress is Ti50's 7-bit I2C address (0x50 = decimal 80).
const TpmI2cAddress = "80"

// TpmTi50DidVidValue is the value of the DID_VID register used by Ti50.
var TpmTi50DidVidValue = []byte{0x66, 0x66, 0x4a, 0x50}

// TpmCr50DidVidValue is the value of the DID_VID register used by Cr50.
var TpmCr50DidVidValue = []byte{0xe0, 0x1a, 0x28, 0x00}

const (
	// EmptyPassword is blank password used for authentication
	EmptyPassword = ""
	// RootPlatformHandle is the Platform Root TPM Handle
	RootPlatformHandle tpmutil.Handle = 0x4000000c
	// KernelFileID is the NVMem ID for the kernel file
	KernelFileID tpmutil.Handle = 0x01001008
	// FwmpFileID is the NVMem ID for the Firmware Management Parameters file
	FwmpFileID tpmutil.Handle = 0x100100a
	// KernelFileAttr is the attribute set that AP firmware uses when creating kernel file
	KernelFileAttr = tpm2.AttrPlatformCreate |
		tpm2.AttrAuthRead |
		tpm2.AttrPPRead |
		tpm2.AttrWriteSTClear |
		tpm2.AttrPPWrite
	// FwmpFileAttr is the attribute set that AP firmware uses when creating FWMP file
	FwmpFileAttr = tpm2.AttrPlatformCreate |
		tpm2.AttrOwnerWrite |
		tpm2.AttrAuthRead |
		tpm2.AttrPPRead |
		tpm2.AttrPPWrite
)

// TpmHandle allows interacting with GSC's TPM bus with higher level tpm commands until tpm2 lib
type TpmHandle struct {
	b        DevBoard
	Ctx      context.Context
	Bus      TpmBus
	response []byte // Contains the response to the last issued request.
}

// NewTpmHandle create a new TpmHandle that can be used with tpm2 library
func NewTpmHandle(ctx context.Context, b DevBoard, bus TpmBus) *TpmHandle {
	return &TpmHandle{b: b, Ctx: ctx, Bus: bus}
}

// OpenTitanToolTpmCommand runs one of the OpenTitanTool TPM subcommands (read-register or execute-command).
func (t *TpmHandle) OpenTitanToolTpmCommand(subcmd string, subargs ...string) ([]byte, error) {
	var args []string
	if t.Bus == TpmBusI2c {
		args = append(args, "--addr", TpmI2cAddress)
	}
	args = append(args, "tpm", subcmd)
	args = append(args, subargs...)
	response, err := t.b.OpenTitanToolCommand(t.Ctx, string(t.Bus), args...)
	if err != nil {
		return nil, err
	}
	b, err := hex.DecodeString(response["hexdata"].(string))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Execute sends a TPM request using possibly multiple writes to the FIFO and status
// registers, and waits for the execution to complete before retrieving the reply. Only use this
// if the Tpm interface does not provided access, e.g. VendorCommands
func (t *TpmHandle) Execute(request []byte) ([]byte, error) {
	response, err := t.OpenTitanToolTpmCommand("execute-command", "--hexdata", string(hex.EncodeToString(request)))
	if err != nil {
		return nil, err
	}
	return response, nil
}

// Write will be called by the go-tpm library to send a command to the TPM.
func (t *TpmHandle) Write(data []byte) (int, error) {
	response, err := t.Execute(data)
	if err != nil {
		return 0, err
	}
	t.response = response
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
