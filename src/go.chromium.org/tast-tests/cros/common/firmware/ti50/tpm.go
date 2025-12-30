// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

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

// TpmOTDidVidValue is the value of the DID_VID register used by OpenTitan.
var TpmOTDidVidValue = []byte{0x66, 0x66, 0x66, 0x50}

// TpmDTDidVidValue is the value of the DID_VID register used by DT.
var TpmDTDidVidValue = []byte{0x66, 0x66, 0x4a, 0x50}

// TpmH1DidVidValue is the value of the DID_VID register used by H1.
var TpmH1DidVidValue = []byte{0xe0, 0x1a, 0x28, 0x00}

const (
	// RootPlatformHandle is the Platform Root TPM Handle
	RootPlatformHandle tpm2.TPMHandle = 0x4000000c
	// KernelNvIndex is the NVMem ID for the kernel file
	KernelNvIndex tpm2.TPMHandle = 0x01001008
	// EncstatefulNvIndex is the NVMem ID for the Encrypted Stateful space
	EncstatefulNvIndex tpm2.TPMHandle = 0x1800005
	// FwmpNvIndex is the NVMem ID for the Firmware Management Parameters file
	FwmpNvIndex tpm2.TPMHandle = 0x100100a
	// WideVineRotIndex is the NVMem ID for extended the Widevine ROT space
	// which could in addition to the ROT seed also contain HDCP and GSC
	// Counter seeds.
	WideVineRotIndex tpm2.TPMHandle = 0x013fff05
)

// TpmHandle allows interacting with GSC's TPM bus with higher level tpm commands until tpm2 lib
type TpmHandle struct {
	b   DevBoard
	Ctx context.Context
	Bus TpmBus
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
	ctx, cancel := context.WithTimeout(t.Ctx, 2*time.Minute)
	defer cancel()
	response, err := t.b.OpenTitanToolCommand(ctx, string(t.Bus), args...)
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

// TpmvInvalidateInactiveRW invalidates the gsc image in the inactive RW.
func (t *TpmHandle) TpmvInvalidateInactiveRW() error {
	var tpmvInvalidateInactiveRW, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0014") // subcommand: InvalidateInactiveRW

	response, err := t.Send(tpmvInvalidateInactiveRW)
	if err != nil {
		return err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("InvalidateInactiveRW command returned error: 0x%x", errorCode)
	}
	return nil
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
	msStr := fmt.Sprintf("%04x", ms)
	tpmvReboot, err := hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000e" + // size
		"20000000" + // ordinal: vendor
		"0013" + // subcommand: ImmediateReset
		msStr)
	if err != nil {
		return err
	}

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

// EncstatefulAttr generates the public area for the FWMP NV index.
func EncstatefulAttr() tpm2.TPMSNVPublic {
	return tpm2.TPMSNVPublic{
		NVIndex: EncstatefulNvIndex,
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

// TpmvSetBoardID sets the board id.
func (t *TpmHandle) TpmvSetBoardID(boardIDType, boardIDFlags BIDField) error {
	boardIDTypeStr := fmt.Sprintf("%08x", boardIDType)
	boardIDFlagsStr := fmt.Sprintf("%08x", boardIDFlags)

	var tpmvSetBoardID, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"00000014" + // size
		"20000000" + // ordinal: vendor
		"001a" + // subcommand: SetBoardID
		boardIDTypeStr +
		boardIDFlagsStr)

	response, err := t.Send(tpmvSetBoardID)
	if err != nil {
		return err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("SetBoardID command returned error: 0x%x", errorCode)
	}
	return nil
}

// TpmvGetFactoryConfig reads the factory config via vendor command.
func (t *TpmHandle) TpmvGetFactoryConfig() (uint64, error) {
	var tpmvSetFactoryConfig, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" + // size
		"20000000" + // ordinal: vendor
		"0044") // subcommand: SetFactoryConfig

	response, err := t.Send(tpmvSetFactoryConfig)
	if err != nil {
		return 0, err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return 0, err
	}
	if errorCode != 0 {
		return 0, errors.Errorf("GetFactoryConfig command returned error: 0x%x", errorCode)
	}

	if len(response) < 20 {
		return 0, errors.Errorf("GetFactoryConfig response not large enough: %v", response)
	}
	return binary.BigEndian.Uint64(response[12:20]), nil
}

// TpmvSetFactoryConfig reads the factory config via vendor command.
func (t *TpmHandle) TpmvSetFactoryConfig(config uint64) (uint32, error) {
	configStr := fmt.Sprintf("%016x", config)
	var tpmvSetFactoryConfig, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"00000014" + // size
		"20000000" + // ordinal: vendor
		"0045" + // subcommand: SetFactoryConfig
		configStr)

	response, err := t.Send(tpmvSetFactoryConfig)
	if err != nil {
		return 0, err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return 0, err
	}
	return errorCode, nil
}

// TpmvTurnUpdateOn sends the vendor command to turn on the pending update.
func (t *TpmHandle) TpmvTurnUpdateOn(delay uint16) error {
	delayStr := fmt.Sprintf("%04x", delay)
	var tpmvTurnUpdateOn, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000e" + // size
		"20000000" + // ordinal: vendor
		"0018" + // subcommand: TurnUpdateOn
		delayStr)

	response, err := t.Send(tpmvTurnUpdateOn)
	if err != nil {
		return err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("TurnUpdateOn command returned error: 0x%x", errorCode)
	}
	return nil
}

// TpmvSetSNBits sets the GSC serial number.
func (t *TpmHandle) TpmvSetSNBits(sn []byte) error {
	sizeStr := fmt.Sprintf("%08x", 12+len(sn))
	var setSNBitsHeader, err = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		sizeStr +
		"20000000" + // ordinal vendor
		"0029") // subcommand: set SN_BITS
	if err != nil {
		return err
	}
	tpmvSetSNBits := append(setSNBitsHeader, sn...)
	response, err := t.Send(tpmvSetSNBits)
	if err != nil {
		return err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("SetSNBits command returned error: 0x%x", errorCode)
	}
	return nil
}

// TpmvFactoryModeDisable sends the vendor command to disable factory mode
func (t *TpmHandle) TpmvFactoryModeDisable() error {
	var tpmvFactoryDisable, err = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000c" +
		"20000000" + // ordinal vendor
		"0020") // subcommand: Factory Disable
	if err != nil {
		return err
	}
	response, err := t.Send(tpmvFactoryDisable)
	if err != nil {
		return err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("FactoryModeDisable command returned error: 0x%x", errorCode)
	}
	return nil
}

// TpmvSetStrongboxState sends the vendor command to enable or disable Strongbox.
func (t *TpmHandle) TpmvSetStrongboxState(enable bool) error {
	enableStr := "00"
	if enable {
		enableStr = "01"
	}
	var tpmvSetStrongboxState, _ = hex.DecodeString("8001" + // tag: TPM_ST_NO_SESSIONS
		"0000000d" + // size
		"20000000" + // ordinal: vendor
		"0050" + // subcommand: SetStrongboxState
		enableStr)

	response, err := t.Send(tpmvSetStrongboxState)
	if err != nil {
		return err
	}
	errorCode, err := getTPMVResponseStatus(response)
	if err != nil {
		return err
	}
	if errorCode != 0 {
		return errors.Errorf("SetStrongboxState command returned error: 0x%x", errorCode)
	}
	return nil
}

const (
	// ExtendDevBoot is the value extended into PCR0 for dev mode (rec=0, dev=1) - SHA1(0x01|0x00|0x01) + 0s to SHA256 size
	ExtendDevBoot = "c42ac1c46f1d4e211c735cc7dfad4ff8391110e9000000000000000000000000"
	// DigestDevBoot is the PCR0 digest value in dev mode
	DigestDevBoot = "23E14DD9BB51A50E16911F7E11DF1E1AAF0B17134DC739C5653607A1EC8DD37A"
	// ExtendNormalBoot is the value extended into PCR0 for normal mode (rec=0, dev=0) - SHA1(0x00|0x00|0x01) + 0s to SHA256 size
	ExtendNormalBoot = "2547cc736e951fa4919853c43ae890861a3b3264000000000000000000000000"
	// DigestNormalBoot is the PCR0 digest value in normal mode
	DigestNormalBoot = "89EAF35134B4B3C649F44C0C765B96AEAB8BB34EE83CC7A683C4E53D1581C8C7"
	// ExtendRecBoot is the value extended into PCR0 for recovery mode (rec=1, dev=0) - SHA1(0x00|0x01|0x00) + 0s to SHA256 size
	ExtendRecBoot = "62571891215b4efc1ceab744ce59dd0b66ea6f73000000000000000000000000"
	// DigestRecBoot is the PCR0 digest value in rec mode
	DigestRecBoot = "9F9EA866D3F34FE3A3112AE9CB1FBABC6FFE8CD261D42493BC6842A9E4F93B3D"
	// ExtendRecDevBoot is the value extended into PCR0 for recovery + dev mode (rec=1, dev=1) -  SHA1(0x01|0x01|0x00) + 0s to SHA256 size
	ExtendRecDevBoot = "47ec8d98366433dc002e7721c9e37d5067547937000000000000000000000000"
	// DigestRecDevBoot is the PCR0 digest value in rec + dev mode
	DigestRecDevBoot = "2A7580E5DA289546F4D2E0509CC6DE155EA131818954D36D49E027FD42B8C8F8"
	// ZeroPCR is the uninitialized PCR0 value
	ZeroPCR = "0000000000000000000000000000000000000000000000000000000000000000"

	// ExtendUnknownBoot is a non-standard value. Used to test unknown boot modes
	ExtendUnknownBoot = "1000000000000000000000000000000000000000000000000000000000000000"
	// DigestUnknownBoot is the digest for the UnknownBoot extended value
	DigestUnknownBoot = "a44a029e04493b8d2fe7893391c2b3ceefec1603c585aad6203f2d14e07bfead"
)

// PCRRead reads the contents of the given PCR
func (t *TpmHandle) PCRRead(pcr uint8) (*tpm2.PCRReadResponse, error) {
	if pcr > 7 {
		return nil, errors.Errorf("invalid PCR: %d", pcr)
	}
	pcrSelect := []byte{1 << pcr, 0x00, 0x00}
	pcrRead := tpm2.PCRRead{
		PCRSelectionIn: tpm2.TPMLPCRSelection{
			PCRSelections: []tpm2.TPMSPCRSelection{
				{
					Hash:      tpm2.TPMAlgSHA256,
					PCRSelect: pcrSelect,
				},
			},
		},
	}
	return pcrRead.Execute(t)
}

// PCRExtend extends the value into the given PCR
func (t *TpmHandle) PCRExtend(pcr uint8, extendDigest string) error {
	extendBytes, err := hex.DecodeString(extendDigest)
	if err != nil {
		return errors.Wrap(err, "failed to decode extend value")
	}
	authHandle := tpm2.AuthHandle{
		Handle: tpm2.TPMHandle(pcr),
		Auth:   tpm2.PasswordAuth(nil),
	}
	pcrExtend := tpm2.PCRExtend{
		PCRHandle: authHandle,
		Digests: tpm2.TPMLDigestValues{
			Digests: []tpm2.TPMTHA{
				{
					HashAlg: tpm2.TPMAlgSHA256,
					Digest:  extendBytes,
				},
			},
		},
	}
	if _, err := pcrExtend.Execute(t); err != nil {
		return errors.Wrapf(err, "PCR%d extend failed", pcr)
	}
	return nil
}

// PCRExtendCheckDigest extends PCR and checks that the new value it reads matches the expected digest
func (t *TpmHandle) PCRExtendCheckDigest(pcr uint8, extendDigest, expectedDigest string) error {
	expectedBytes, err := hex.DecodeString(expectedDigest)
	if err != nil {
		return errors.Wrap(err, "failed to decode expected digest")
	}
	err = t.PCRExtend(pcr, extendDigest)
	if err != nil {
		return errors.Wrapf(err, "failed to extend PCR%d", pcr)
	}
	read, err := t.PCRRead(pcr)
	if err != nil {
		return errors.Wrapf(err, "PCR%d read failed", pcr)
	}
	if !bytes.Equal(read.PCRValues.Digests[0].Buffer, expectedBytes) {
		return errors.Wrapf(err, "PCR%d did not match the expected value", pcr)
	}
	return nil
}

// GetTPMProperty gets the specified property value
func (t *TpmHandle) GetTPMProperty(prop tpm2.TPMPT) (uint32, error) {
	getCap := tpm2.GetCapability{
		Capability:    tpm2.TPMCapTPMProperties,
		Property:      uint32(prop),
		PropertyCount: 1,
	}

	response, err := getCap.Execute(t)
	if err != nil {
		return 0, errors.Wrapf(err, "TPMGetCapability for %d failed", prop)
	}
	property, err := response.CapabilityData.Data.TPMProperties()
	if err != nil {
		return 0, errors.Wrapf(err, "no property available for %d", prop)
	}
	for _, p := range property.TPMProperty {
		if p.Property == prop {
			return p.Value, nil
		}
	}

	return 0, errors.Errorf("property not found %d", prop)
}
