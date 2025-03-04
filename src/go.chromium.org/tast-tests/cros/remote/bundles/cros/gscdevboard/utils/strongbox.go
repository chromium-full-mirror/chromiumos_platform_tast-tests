// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/binary"

	"go.chromium.org/tast/core/errors"
)

// StrongboxCmd type is 2 bytes
type StrongboxCmd uint16

// Strongbox command codes
const (
	StrongboxDeviceGetHardwareInfo StrongboxCmd = 0x11
	StrongboxDeviceAddRngEntropy   StrongboxCmd = 0x12
	StrongboxDeviceGenerateKey     StrongboxCmd = 0x13
	StrongboxDeviceImportKey       StrongboxCmd = 0x14
)

// StrongboxStatus type is 4 bytes
type StrongboxStatus uint32

// Strongbox response status codes
const (
	StrongboxSuccess       StrongboxStatus = 0
	InvalidArgument        StrongboxStatus = 0x400 + 38
	UnsupportedTag         StrongboxStatus = 0x400 + 39
	InvalidTag             StrongboxStatus = 0x400 + 40
	StrongboxUnimplemented StrongboxStatus = 0x400 + 100
)

const strongboxTpmVendorCommand uint32 = 0x20000001

// StrongboxCommand sends a command and returns the response.
func StrongboxCommand(ctx context.Context, tpm *TpmHelper, command StrongboxCmd) (status StrongboxStatus, response []byte, err error) {
	var buf []byte
	buf = binary.BigEndian.AppendUint16(buf, 0x8001) // TPM_ST_NO_SESSIONS
	buf = binary.BigEndian.AppendUint32(buf, 10)     // size
	buf = binary.BigEndian.AppendUint32(buf, strongboxTpmVendorCommand)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(command))
	response, err = tpm.Send(buf)
	if err != nil {
		return status, response, errors.Wrap(err, "failed to send")
	}
	if len(response) < 10 {
		return status, response, errors.Errorf("Response too small: %v", response)
	}
	status = StrongboxStatus(binary.BigEndian.Uint32(response[6:10]))
	return status, response[10:], nil
}
