// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mcci provides control to an MCCI switch connected to the host.
// Currently supported/tested models:
// - MCCI 3141 (https://store.mcci.com/products/model3141)
package mcci

import (
	"fmt"

	"go.bug.st/serial"

	"go.chromium.org/tast/core/errors"
)

// Default device path at which the MCCI switch serial device is always mounted.
const (
	mcciPortPath = "/dev/ttyACM0"
)

// Switch is the external facing struct that represents an MCCI switch.
type Switch struct {
	sPort serial.Port
}

// GetSwitch returns a handle to the MCCI switch.
func GetSwitch() (*Switch, error) {
	mode := &serial.Mode{BaudRate: 9600, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
	port, err := serial.Open(mcciPortPath, mode)
	if err != nil {
		return nil, errors.Wrap(err, "unable to open MCCI serial port")
	}

	return &Switch{sPort: port}, nil
}

// DisablePorts disables all ports.
func (sw Switch) DisablePorts() error {
	return writeSerial("port 0\r", sw.sPort)
}

// EnablePort enables the port `portNum`.
func (sw Switch) EnablePort(portNum int) error {
	if portNum != 1 && portNum != 2 {
		return errors.New("invalid port number provided")
	}

	serialStr := fmt.Sprintf("port %d\r", portNum)
	return writeSerial(serialStr, sw.sPort)
}

// Close closes the serial port interface for the MCCI switch.
func (sw Switch) Close() {
	sw.sPort.Close()
}

// writeSerial implements the raw write command to the MCCI serial interface.
func writeSerial(str string, port serial.Port) error {
	if _, err := port.Write([]byte(str)); err != nil {
		return errors.Wrap(err, "Unable to write to MCCI serial port")
	}

	return nil
}
