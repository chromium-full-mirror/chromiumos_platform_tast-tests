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

// InitFixture initializes fixtures.
func InitFixture(ctx context.Context) error {
	// Due to the hardware layout design. The fixtures are connected to the
	// USB hub, and the USB hub's power is connected to the 4th port of IP
	// power. Need to ensure it has been turned on when the test starts.
	ippowerPorts := []int{USBHubPort}
	if err := OpenIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "open the 4th port of the IP power supply")
	}

	ports, err := serial.GetPortsList()
	if err != nil {
		return errors.Wrap(err, "failed to get port list")
	}

	if len(ports) == 0 {
		return errors.New("no serial ports found")
	}

	// GoBigSleepLint: Prevent switch fixture function not in time.
	testing.Sleep(ctx, 3*time.Second)

	// Print the list of detected ports.
	for _, port := range ports {
		// Open the first serial port detected at 9600bps N81.
		mode := &serial.Mode{
			BaudRate: 9600,
		}

		usbPort, err := serial.Open(port, mode)
		if err != nil {
			return errors.Wrap(err, "serial open error")
		}

		var t = 3 * time.Second
		usbPort.SetReadTimeout(t)

		_, err = usbPort.Write([]byte("i"))
		if err != nil {
			return errors.Wrap(err, "serial write error")
		}

		// Read and print the response.
		buff := make([]byte, 1000)
		for {
			// Reads up to 1000 bytes.
			n, err := usbPort.Read(buff)

			if err != nil {
				return errors.Wrap(err, "serial read error")
			}

			if n == 0 {
				fmt.Println("\nEOF")
				break
			}

			mString := string(buff[:n])
			mString = strings.Replace(mString, "\r", "", -1)
			res := strings.Split(mString, "\n")

			for _, s := range res {
				if len(s) > fixtureIDLen && strings.Count(s[0:15], "_") == 2 && strings.Count(s[0:15], " ") == 0 {
					serial := s[0:15]
					fixtureID := fmt.Sprintf("test fixture id: %s => port: %s", serial, port)
					testing.ContextLog(ctx, fixtureID)

					uid, found := fixtureUID[serial]

					if found {
						fixtureOnline[uid] = port
					}
				}
			}
		}
		usbPort.Close()
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
		// Open the serial port detected at 9600bps.
		mode := &serial.Mode{
			BaudRate: 9600,
		}

		usbPort, err := serial.Open(port, mode)
		if err != nil {
			return errors.Wrapf(err, "open uid:%s error", uid)
		}
		defer usbPort.Close()
		var t = 3 * time.Second
		usbPort.SetReadTimeout(t)

		_, err = usbPort.Write([]byte("i"))
		if err != nil {
			return errors.Wrapf(err, "write uid:%s error", uid)
		}

		// Read and print the response.
		buff := make([]byte, 1000)
		for {
			// Reads up to 1000 bytes.
			n, err := usbPort.Read(buff)

			if err != nil {
				return errors.Wrapf(err, "uid:%s read error", uid)
			}

			if n == 0 {
				break
			}
			mString := string(buff[:n])
			mString = strings.Replace(mString, "\r", "", -1)
			res := strings.Split(mString, "\n")

			for _, s := range res {
				if len(s) > fixtureIDLen && strings.Count(s[0:15], "_") == 2 && strings.Count(s[0:15], " ") == 0 {
					break
				} else {
					return errors.Errorf("failed to read uid:%s info", uid)
				}
			}
		}
	}
	return nil
}

// ControlFixture is for control fixture.
func ControlFixture(ctx context.Context, uid, cmd string) error {
	s := fmt.Sprintf("Fixture '%s' set '%s' ", uid, cmd)
	testing.ContextLog(ctx, s)

	port, found := fixtureOnline[uid]

	if !found {
		s := fmt.Sprintf("uid %s is not in online fixture list.", uid)
		return errors.New(s)
	}

	mode := &serial.Mode{
		BaudRate: 9600,
	}

	usbPort, err := serial.Open(port, mode)
	if err != nil {
		return errors.Wrap(err, "serial open error")
	}

	var t time.Duration = 3 * time.Second
	usbPort.SetReadTimeout(t)

	cmdNum := fixtureCmd[uid][cmd]

	_, err = usbPort.Write([]byte(cmdNum))
	if err != nil {
		return errors.Wrap(err, "serial write error")
	}

	usbPort.Close()

	// GoBigSleepLint: Prevent switch fixture function not in time.
	testing.Sleep(ctx, 7*time.Second)

	return nil
}

// OpenAllFixture is for open all fixture.
func OpenAllFixture(ctx context.Context) {
	for uid := range fixtureOnline {
		ControlFixture(ctx, uid, "on")
	}
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

// PrintAllFixture is for print all fixture on context log.
func PrintAllFixture(ctx context.Context) {
	for uid, port := range fixtureOnline {
		s := fmt.Sprintf("print all fixture id: %s => port: %s \n", uid, port)
		testing.ContextLog(ctx, s)
	}
}

// GetOnlineDisplayFixture is for getting online display fixture
func GetOnlineDisplayFixture() map[string]string {

	displayFixtureOnline := make(map[string]string)

	for uid, port := range fixtureOnline {
		if fixtureIsDisplay[uid] {
			displayFixtureOnline[uid] = port
		}
	}

	return displayFixtureOnline
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
