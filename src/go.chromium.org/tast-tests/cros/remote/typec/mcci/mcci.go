// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mcci provides control to an MCCI switch connected to the host.
// Currently supported/tested models:
// - MCCI 3141 (https://store.mcci.com/products/model3141)
package mcci

import (
	"fmt"

	"github.com/tarm/serial"

	"go.chromium.org/tast/core/errors"
)

// Default device path at which the MCCI switch serial device is always mounted.
const (
	mcciPortPath = "/dev/ttyACM0"
)

// DisablePorts disables all ports.
func DisablePorts() error {
	return writeSerial("port 0\r")
}

// EnablePort enables the port `portNum`.
func EnablePort(portNum int) error {
	if portNum != 1 && portNum != 2 {
		return errors.New("invalid port number provided")
	}

	serialStr := fmt.Sprintf("port %d\r", portNum)
	return writeSerial(serialStr)
}

// writeSerial implements the raw write command to the MCCI serial interface.
func writeSerial(str string) error {
	// Rather than maintaining an open connection, it's simpler to just open the port each time a write occurs and close it immediately after.
	c := &serial.Config{Name: mcciPortPath, Baud: 9600, Parity: serial.ParityNone, StopBits: serial.Stop1}
	port, err := serial.OpenPort(c)
	if err != nil {
		return errors.Wrap(err, "Unable to open MCCI serial port")
	}
	defer port.Close()

	if _, err := port.Write([]byte(str)); err != nil {
		return errors.Wrap(err, "Unable to write to MCCI serial port")
	}

	return nil
}
