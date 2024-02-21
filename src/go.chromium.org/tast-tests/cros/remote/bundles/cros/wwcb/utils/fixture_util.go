// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// wwcbPowerCycleFixture is a variable to enable to power cycle fixture in the WWCB tests.
var wwcbPowerCycleFixture = testing.RegisterVarString(
	"utils.wwcbPowerCycleFixture",
	"off",
	"A variable to enable to power cycle fixture in the WWCB tests",
)

var (
	fixtureUID = map[string]string{
		"AUS19129_C01_01": "1912901",
		"AUS19129_C01_02": "1912902",

		"AHS20079_A00_01": "2007901",
		"AHS20079_A00_02": "2007902",

		"AUS20019_D00_01": "2001901",
		"AUS20019_D00_02": "2001902",
		"AUS20019_D00_03": "2001903",
		"AUS20019_D00_04": "2001904",
		"AUS20019_D00_05": "2001905",

		"ADT21090_B00_01": "2109001",
		"ADT21090_B00_02": "2109002",

		"XXRJ45SW_X00_01": "j45sw01",
	}

	fixtureIsDisplay = map[string]bool{
		"1912901": true,
		"1912902": true,
		"2007901": true,
		"2007902": true,
		"2001901": false,
		"2001902": false,
		"2001903": false,
		"2001904": false,
		"2001905": false,
		"2109001": true,
		"2109002": true,
		"j45sw01": false,
	}

	fixtureCmd = map[string]map[string]string{
		"1912901": {"off": "0", "on": "1", "flip": "2"},
		"1912902": {"off": "0", "on": "1", "flip": "2"},
		"2007901": {"off": "2", "on": "1"},
		"2007902": {"off": "2", "on": "1"},
		"2001901": {"off": "2", "on": "1"},
		"2001902": {"off": "2", "on": "1"},
		"2001903": {"off": "2", "on": "1"},
		"2001904": {"off": "2", "on": "1"},
		"2001905": {"off": "2", "on": "1"},
		"2109001": {"off": "3", "on": "1"},
		"2109002": {"off": "3", "on": "1"},
		"j45sw01": {"off": "2", "on": "1"},
	}

	// key:text fixture uid , value:usb port id
	fixtureOnline = make(map[string]string)

	// fixture ID length
	fixtureIDLen = 22

	// USBHubPort is assigned to the 4th port of the IP power supply.
	USBHubPort = 4
)

// requestSerialPort returns a response after sends a request to the serial port.
func requestSerialPort(ctx context.Context, port, req string) (string, error) {
	mode := &serial.Mode{
		BaudRate: 9600,
	}
	usbPort, err := serial.Open(port, mode)
	if err != nil {
		return "", errors.Wrap(err, "open serial port")
	}
	defer usbPort.Close()

	if err := usbPort.SetReadTimeout(3 * time.Second); err != nil {
		return "", errors.Wrap(err, "set read timeout")
	}

	if _, err := usbPort.Write([]byte(req)); err != nil {
		return "", errors.Wrap(err, "write serial port")
	}

	// Read the response.
	var resp string
	buff := make([]byte, 1000)
	for {
		n, err := usbPort.Read(buff)
		if err != nil {
			return "", errors.Wrap(err, "read serial port")
		}
		if n == 0 {
			fmt.Println("\nEOF")
			break
		}

		resp = strings.TrimSpace(string(buff[:n]))
	}

	return resp, nil
}

// InitFixture stores each detected fixture in a list and sets the fixtures status to off.
func InitFixture(ctx context.Context) error {
	ports, err := serial.GetPortsList()
	if err != nil {
		return errors.Wrap(err, "failed to get port list")
	}
	if len(ports) == 0 {
		return errors.New("no serial ports found")
	}

	// Retrieve the fixture information from each serial port.
	for _, port := range ports {
		// Only use serial ports containing ACM*, e.g. /dev/ttyACM0
		if !strings.Contains(port, "ACM") {
			continue
		}
		resp, err := requestSerialPort(ctx, port, "i")
		if err != nil {
			return errors.Wrap(err, "request serial port")
		}

		for _, s := range strings.Split(resp, "\n") {
			if len(s) > fixtureIDLen && strings.Count(s[0:15], "_") == 2 && strings.Count(s[0:15], " ") == 0 {
				serial := s[0:15]
				testing.ContextLogf(ctx, "test fixture id: %s => port: %s", serial, port)

				uid, found := fixtureUID[serial]
				if found {
					fixtureOnline[uid] = port
				}
			}
		}
	}

	// Since all fixtures are considered off at the beginning of each test.
	// Need to ensure all fixtures are switched off when the test starts.
	if err := CloseAllFixture(ctx); err != nil {
		return errors.Wrap(err, "failed to turn off all fixtures")
	}

	return nil
}

// TestAllFixtures tests all fixtures are alive.
func TestAllFixtures(ctx context.Context) error {
	for uid, port := range fixtureOnline {
		resp, err := requestSerialPort(ctx, port, "i")
		if err != nil {
			return errors.Wrap(err, "request serial port")
		}

		for _, s := range strings.Split(resp, "\n") {
			if len(s) > fixtureIDLen && strings.Count(s[0:15], "_") == 2 && strings.Count(s[0:15], " ") == 0 {
				break
			} else {
				return errors.Errorf("failed to read uid:%s info", uid)
			}
		}
	}

	return nil
}

// ControlFixture sets the fixture status to on or off.
func ControlFixture(ctx context.Context, uid, status string) error {
	testing.ContextLogf(ctx, "Set fixture %q to %q", uid, status)

	port, found := fixtureOnline[uid]
	if !found {
		return errors.Errorf("Unable to find the serial port of the fixture uid: %s", uid)
	}

	cmd, found := fixtureCmd[uid][status]
	if !found {
		return errors.Errorf("Unable to find the correspond command of the fixture status: %s", status)
	}

	if _, err := requestSerialPort(ctx, port, cmd); err != nil {
		return errors.Wrap(err, "request serial port")
	}

	return nil
}

// CloseAllFixture turns off all fixtures connected to the host and powers cycle it according to the
// input parameter.
func CloseAllFixture(ctx context.Context) error {
	for uid, port := range fixtureOnline {
		if err := ControlFixture(ctx, uid, "off"); err != nil {
			return errors.Wrapf(err, "failed to control %s to turn off on port %s", uid, port)
		}
	}

	if wwcbPowerCycleFixture.Value() == "on" {
		if err := PowerCycleFixture(ctx); err != nil {
			return errors.Wrap(err, "failed to power cycle fixture")
		}
	}

	return nil
}

// PowerCycleFixture Turns the IP power supply's 4th port (assigned to the USB hub) off and on to power cycle the fixtures.
func PowerCycleFixture(ctx context.Context) error {
	ippowerPorts := []int{USBHubPort}
	if err := CloseIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "close the 4th port of the IP power supply")
	}
	// GoBigSleepLint: Sleep for a second before power on USB hub.
	testing.Sleep(ctx, 1*time.Second)
	if err := OpenIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "open the 4th port of the IP power supply")
	}
	return nil
}
