// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mcci provides control to an MCCI switch connected to the host.
// Currently supported/tested models:
// - MCCI 3141 (https://store.mcci.com/products/model3141)
package mcci

import (
	"fmt"
	"regexp"
	"strings"
	"time"

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

// GetSwitch returns a handle to the MCCI switch with serial number `serialNum`.
func GetSwitch(serialNum string) (*Switch, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve serial ports list")
	}

	if len(ports) == 0 {
		return nil, errors.Wrap(err, "no serial ports found")
	}

	for _, portPath := range ports {
		mode := &serial.Mode{BaudRate: 9600, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
		port, err := serial.Open(portPath, mode)
		if err != nil {
			// If we aren't able to open the port, just continue.
			continue
		}

		if match := checkPort(port, serialNum); match {
			return &Switch{sPort: port}, nil
		}

		port.Close()
	}

	return nil, errors.Wrapf(err, "couldn't find MCCI switch with serial number %s", serialNum)
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

// checkPort is a helper function that checks if the MCCI switch (represented by its serial port `port` matches the supplied serial number.
func checkPort(port serial.Port, serialNum string) bool {
	if err := writeSerial("sn\r", port); err != nil {
		return false
	}

	resultStr, err := readSerial(port)
	if err != nil {
		return false
	}

	serialRegex := regexp.MustCompile(`Serial number: (\S+)`)

	if match := serialRegex.FindStringSubmatch(resultStr); match != nil {
		if strings.Compare(string(match[1]), serialNum) == 0 {
			return true
		}
	}

	return false
}

// readSerial implements a read command to the MCCI serial interface and returns the buffer in the form of a string. In the case of an empty read, it returns an empty string.
func readSerial(port serial.Port) (string, error) {
	port.SetReadTimeout(20 * time.Millisecond)

	var result = ""
	for {
		buff := make([]byte, 1000)
		n, err := port.Read(buff)

		if err != nil {
			return result, errors.Wrap(err, "serial read error")
		}

		if n == 0 {
			break
		}

		result = result + string(buff[:n])
	}

	return result, nil
}

// writeSerial implements the raw write command to the MCCI serial interface.
func writeSerial(str string, port serial.Port) error {
	if _, err := port.Write([]byte(str)); err != nil {
		return errors.Wrap(err, "Unable to write to MCCI serial port")
	}

	return nil
}
