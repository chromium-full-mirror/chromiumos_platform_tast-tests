// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"encoding/binary"
	"encoding/hex"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast/core/errors"
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

	// TpmBusInvalid is an "invalid" enum value of type TpmBus.
	TpmBusInvalid TpmBus = "invalid"
)

// TpmI2cAddress is Ti50's 7-bit I2C address (0x50 = decimal 80).
const TpmI2cAddress = "80"

// TpmTi50DidVidValue is the value of the DID_VID register used by Ti50.
var TpmTi50DidVidValue = []byte{0x66, 0x66, 0x4a, 0x50}

// TpmCr50DidVidValue is the value of the DID_VID register used by Cr50.
var TpmCr50DidVidValue = []byte{0xe0, 0x1a, 0x28, 0x00}

const (
	// RootPlatformHandle is the Platform Root TPM Handle
	RootPlatformHandle tpm2.TPMHandle = 0x4000000c
	// KernelNvIndex is the NVMem ID for the kernel file
	KernelNvIndex tpm2.TPMHandle = 0x01001008
	// FwmpNvIndex is the NVMem ID for the Firmware Management Parameters file
	FwmpNvIndex tpm2.TPMHandle = 0x100100a
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
	args = append(args, "tpm", "--gsc-ready", string(GpioTi50ApIntL), subcmd)
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

// Send sends a TPM request using possibly multiple writes to the FIFO and status
// registers, and waits for the execution to complete before retrieving the reply. Only use this
// if the Tpm interface does not provided access, e.g. VendorCommands
func (t *TpmHandle) Send(request []byte) ([]byte, error) {
	response, err := t.OpenTitanToolTpmCommand("execute-command", "--hexdata", string(hex.EncodeToString(request)))
	if err != nil {
		return nil, err
	}
	return response, nil
}

// NvUndefineSpace undefines the NV space indicated by the passed public area.
func (t *TpmHandle) NvUndefineSpace(p tpm2.TPMSNVPublic) error {
	nvName, err := tpm2.NVName(&p)
	if err != nil {
		return errors.Join(errors.New("failed to get NV name: "), err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: p.NVIndex,
		Name:   *nvName,
	}

	undef := tpm2.NVUndefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
	}
	_, err = undef.Execute(t)
	return err
}

// TpmvGetBootMode reads boot mode via TPM GetBootMode vendor command.
func (t *TpmHandle) TpmvGetBootMode() (byte, error) {
	var tpmvGetBootMode, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0034") // subcommand: GetBootMode

	response, err := t.Send(tpmvGetBootMode)
	if err != nil {
		return 0, err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return 0, err
	}
	if errorCode != 0 {
		return 0, errors.Errorf("GetBootMode command returned error: 0x%x", errorCode)
	}
	mode := response[12]
	return mode, nil
}

// TpmvGetApRoVerificationStatus reads AP RO Verification status via vendor
// command.
func (t *TpmHandle) TpmvGetApRoVerificationStatus() (APROResultCode, error) {
	var tpmvGetBootMode, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0039") // subcommand: GetApRoVerificationStatus

	response, err := t.Send(tpmvGetBootMode)
	if err != nil {
		return ApRoV2Unknown, err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return ApRoV2Unknown, err
	}
	if errorCode != 0 {
		return ApRoV2Unknown, errors.Errorf("GetApRoVerificationStatus command returned error: %d", errorCode)
	}
	mode := response[12]
	return APROResultCode(mode), nil
}

// TpmvCommitNvmem sends the CommitNvmem vendor command.
func (t *TpmHandle) TpmvCommitNvmem() error {
	tpmvCommitNvmem, err := hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0015") // subcommand: CommitNvmem
	if err != nil {
		return err
	}

	response, err := t.Send(tpmvCommitNvmem)
	if err != nil {
		return err
	}

	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("CommitNvmem returned error: 0x%x", errorCode)
	}
	return nil
}

// TpmvReboot sends the reboot vendor command for the specified number of ms.
func (t *TpmHandle) TpmvReboot(ms uint16) error {
	tpmvReboot, err := hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000e" + // size
		"20000000" + // ordinal: vendor
		"0013") // subcommand: ImmediateReset
	if err != nil {
		return err
	}
	// Add ms parameter
	tpmvReboot = binary.BigEndian.AppendUint16(tpmvReboot, ms)

	response, err := t.Send(tpmvReboot)
	if err != nil {
		return err
	}

	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("Reboot command returned error: 0x%x", errorCode)
	}
	return nil
}

func getTPMVResponseStatus(buf []byte) (uint32, error) {
	if len(buf) < 10 {
		return 1, errors.Errorf("TPMV response not large enough: %v", buf)
	}
	return binary.BigEndian.Uint32(buf[6:10]), nil
}

// KernelAttr generates the public area for the kernel NV index.
func KernelAttr() tpm2.TPMSNVPublic {
	return tpm2.TPMSNVPublic{
		NVIndex: KernelNvIndex,
		NameAlg: tpm2.TPMAlgSHA1,
		Attributes: tpm2.TPMANV{
			PlatformCreate: true,
			AuthRead:       true,
			PPRead:         true,
			WriteSTClear:   true,
			PPWrite:        true,
			NT:             tpm2.TPMNTOrdinary,
		},
		DataSize: 40,
	}
}

// FwmpAttr generates the public area for the FWMP NV index.
func FwmpAttr() tpm2.TPMSNVPublic {
	return tpm2.TPMSNVPublic{
		NVIndex: FwmpNvIndex,
		NameAlg: tpm2.TPMAlgSHA1,
		Attributes: tpm2.TPMANV{
			PlatformCreate: true,
			OwnerWrite:     true,
			AuthRead:       true,
			PPRead:         true,
			PPWrite:        true,
			NT:             tpm2.TPMNTOrdinary,
		},
		DataSize: 40,
	}
}

// EmptyPassword generats an empty TPM2BAuth.
func EmptyPassword() tpm2.TPM2BAuth {
	return tpm2.TPM2BAuth{
		Buffer: nil,
	}
}
