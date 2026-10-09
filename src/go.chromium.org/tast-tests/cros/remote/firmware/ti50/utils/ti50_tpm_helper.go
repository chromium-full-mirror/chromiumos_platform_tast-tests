// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strconv"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

// ReadRegisterWithLength reads a TPM register with length supplied by caller.
func (t *TpmHelper) ReadRegisterWithLength(register ti50.TpmRegister, length int) []byte {
	response, err := t.OpenTitanToolTpmCommand("read-register", string(register), "--length", strconv.Itoa(length))
	if err != nil {
		t.h.Fatalf("failed to read TPM register %s: %s", register, err)
	}
	return response
}

// WriteRegister writes a TPM register by communicating via SPI or I2C.
func (t *TpmHelper) WriteRegister(register ti50.TpmRegister, data []byte) {
	_, err := t.OpenTitanToolTpmCommand("write-register", string(register), "--hexdata", string(hex.EncodeToString(data)))
	if err != nil {
		t.h.Fatalf("failed to write TPM register %s: %s", register, err)
	}
}

// SendGo triggers command execution by setting tpmGo (0x20) in TPM_STS register.
func (t *TpmHelper) SendGo() error {
	_, err := t.OpenTitanToolTpmCommand("write-register", string(ti50.TpmRegSts), "--hexdata", "20000000")
	return err
}

// SendCancel sends commandReady (0x40) to TPM_STS register to trigger cooperative cancellation.
func (t *TpmHelper) SendCancel() error {
	_, err := t.OpenTitanToolTpmCommand("write-register", string(ti50.TpmRegSts), "--hexdata", "40000000")
	return err
}

// ReadSts reads the TPM_STS register and returns byte 0 (primary status flags).
func (t *TpmHelper) ReadSts() (byte, error) {
	resp, err := t.OpenTitanToolTpmCommand("read-register", string(ti50.TpmRegSts))
	if err != nil {
		return 0, err
	}
	if len(resp) == 0 {
		return 0, errors.New("empty STS response")
	}
	return resp[0], nil
}

// WriteFifo writes raw command bytes into the TPM DATA_FIFO register.
func (t *TpmHelper) WriteFifo(data []byte) error {
	_, err := t.OpenTitanToolTpmCommand("write-register", string(ti50.TpmRegDataFifo), "--hexdata", hex.EncodeToString(data))
	return err
}

// ReadFifo reads bytes from the TPM DATA_FIFO register.
func (t *TpmHelper) ReadFifo(length int) ([]byte, error) {
	return t.OpenTitanToolTpmCommand("read-register", string(ti50.TpmRegDataFifo), "--length", strconv.Itoa(length))
}

// ToReadyState aborts any command in progress and moves the TPM to the Ready
// state. The first commandReady write moves it from Reception, Execution or
// Completion to Idle, the second one from Idle to Ready.
func (t *TpmHelper) ToReadyState() error {
	for range 2 {
		if err := t.SendCancel(); err != nil {
			return errors.Wrap(err, "failed to write commandReady")
		}
		if _, err := t.ReadSts(); err != nil {
			return errors.Wrap(err, "failed to read TPM_STS")
		}
	}
	return nil
}

// ExecuteRawCommand sends cmd to the TPM through the FIFO registers and returns
// the response header followed by at most maxRead bytes of the response in
// total. Unlike Send, the response doesn't have to be well formed nor read in
// full, which allows checking malformed or oversized responses.
func (t *TpmHelper) ExecuteRawCommand(ctx context.Context, cmd []byte, maxRead int) ([]byte, error) {
	const (
		// Small enough for a single SPI and I2C transaction on all GSCs.
		fifoChunkSize    = 32
		tpmHeaderSize    = 6 // tag + size
		dataAvailBitMask = 0x10
	)

	if err := t.ToReadyState(); err != nil {
		return nil, err
	}
	for chunk := range slices.Chunk(cmd, fifoChunkSize) {
		if err := t.WriteFifo(chunk); err != nil {
			return nil, errors.Wrap(err, "failed to write DATA_FIFO")
		}
	}
	if err := t.SendGo(); err != nil {
		return nil, errors.Wrap(err, "failed to write tpmGo")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		sts, err := t.ReadSts()
		if err != nil {
			return testing.PollBreak(err)
		}
		if sts&dataAvailBitMask == 0 {
			return errors.Errorf("dataAvail not set, TPM_STS 0x%02x", sts)
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Second, Interval: 10 * time.Millisecond}); err != nil {
		return nil, errors.Wrap(err, "failed waiting for response")
	}

	resp, err := t.ReadFifo(tpmHeaderSize)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response header")
	}
	if len(resp) < tpmHeaderSize {
		return nil, errors.Errorf("short response header: %x", resp)
	}
	size := int(binary.BigEndian.Uint32(resp[2:tpmHeaderSize]))
	for want := min(size, maxRead); len(resp) < want; {
		rest, err := t.ReadFifo(min(want-len(resp), fifoChunkSize))
		if err != nil {
			return nil, errors.Wrap(err, "failed to read response")
		}
		if len(rest) == 0 {
			return nil, errors.New("empty DATA_FIFO read")
		}
		resp = append(resp, rest...)
	}
	return resp, nil
}

// RawCaptureTransport captures serialized TPM 2.0 command bytes from tpm2.Command.Execute.
type RawCaptureTransport struct {
	Captured []byte
}

// Send captures the serialized command bytes without transmitting them.
func (r *RawCaptureTransport) Send(cmd []byte) ([]byte, error) {
	r.Captured = make([]byte, len(cmd))
	copy(r.Captured, cmd)
	return nil, nil
}

// SerializeCommand captures the complete marshaled wire command packet from a tpm2.Command.
func SerializeCommand[R any, PR *R](cmd tpm2.Command[R, PR]) []byte {
	var capturer RawCaptureTransport
	_, _ = cmd.Execute(&capturer)
	return capturer.Captured
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
