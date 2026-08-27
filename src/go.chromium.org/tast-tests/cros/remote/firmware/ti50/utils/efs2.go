// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"bytes"
	"context"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"

	"go.chromium.org/tast/core/errors"
)

// Packet preamble and prebuilt response codes
var (
	// Packet preamble
	Efs2Preamble = []byte{0xec, 0xec, 0xec, 0xec, 0xec}
	// Success return packet
	Efs2ReturnSuccess = []byte{0x00, 0xec}
	// Unknown error return packet
	Efs2ReturnErrorUnknown = []byte{0x01, 0xec}
	// Incorrect magic return packet
	Efs2ReturnErrorMagic = []byte{0x02, 0xec}
	// Incorrect crc return packet
	Efs2ReturnErrorCrc = []byte{0x03, 0xec}
	// Incorrect payload size return packet
	Efs2ReturnErrorSize = []byte{0x04, 0xec}
	// Timeout return packet
	Efs2ReturnErrorTimeout = []byte{0x05, 0xec}
	// Invalid command return packet
	Efs2ReturnErrorUndefinedCmd = []byte{0x06, 0xec}
	// Bad payload return packet
	Efs2ReturnErrorBadPayload = []byte{0x07, 0xec}
	// Incorrect version return packet
	Efs2ReturnErrorVersion = []byte{0x08, 0xec}
	// Kernel NV not found or invalid return packet
	Efs2ReturnErrorNvmem = []byte{0x09, 0xec}
	// Invalid parameter return packet
	Efs2ReturnErrorBadParm = []byte{0x0A, 0xec}
)

// Efs2Cmd type is 2 bytes
type Efs2Cmd uint16

// Efs2 command codes
const (
	Efs2CmdSetBootMode Efs2Cmd = 1
	Efs2CmdVerifyHash  Efs2Cmd = 2
)

// Efs2 boot modes
const (
	Efs2BootModeVerified  byte = 0
	Efs2BootModeNoBoot    byte = 1
	Efs2BootModeTrustedRo byte = 2
)

// BootModeToString converts an Efs2 boot mode to a string
func BootModeToString(mode byte) string {
	switch mode {
	case Efs2BootModeVerified:
		return "VERIFIED_RW"
	case Efs2BootModeNoBoot:
		return "NO_BOOT"
	case Efs2BootModeTrustedRo:
		return "TRUSTED_RO"
	default:
		return "UNKNOWN"
	}
}

// CreateEcPacket creates an EFS2 packet with the correct formatting and crc8 data with the
// specified command and payload.
func CreateEcPacket(cmd Efs2Cmd, payload []byte) []byte {
	data := make([]byte, 7+len(payload))
	data[0] = 'E'                // magic
	data[1] = 'C'                // magic
	data[2] = 0                  // version
	data[3] = 0                  // crc -- filled in later
	data[4] = byte(cmd & 0xFF)   // cmd low byte (no command is more than 255)
	data[5] = byte(cmd >> 8)     // cmd high byte
	data[6] = byte(len(payload)) // size
	copy(data[7:], payload)      // payload
	data[3] = Crc8(data[4:])     // crc
	return append(Efs2Preamble, data...)
}

// CheckSendEcPacket sends the specified EC EFS2 packet and tests whether the returned EFS2 code
// is the specified wanted value.
func CheckSendEcPacket(ctx context.Context, b DevboardHelper, ecUart ti50.SerialChannel, packet, want []byte) error {
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	defer b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)

	if err := ecUart.ClearInput(ctx); err != nil {
		return errors.Wrap(err, "could not clear EC serial")
	}
	if err := ecUart.WriteSerial(ctx, packet); err != nil {
		return errors.Wrap(err, "could not write EC serial")
	}
	got, err := ecUart.ReadSerialBytes(ctx, 2)
	if err != nil {
		return errors.Wrap(err, "could not read EC serial")
	} else if !bytes.Equal(got, want) {
		return errors.Errorf("GSC did not respond to EC packet correctly. Got %v, wanted %v", got, want)
	}

	return nil
}

// SendEcPacketNoResponse sends the specified EC EFS2 packet, but does not try to read any response.
// This is useful when the GSC is expected to reset the EC and does not respond with any value.
func SendEcPacketNoResponse(ctx context.Context, b DevboardHelper, ecUart ti50.SerialChannel, packet []byte) error {
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	defer b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)

	if err := ecUart.ClearInput(ctx); err != nil {
		return errors.Wrap(err, "could not clear EC serial")
	}
	if err := ecUart.WriteSerial(ctx, packet); err != nil {
		return errors.Wrap(err, "could not write EC serial")
	}

	return nil
}
