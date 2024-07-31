// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"bytes"
	"context"
	"time"

	"go.bug.st/serial"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	simulatorSerial serial.Port = nil
	simulatorOnline             = ""
)

// InitSimulator initializes simulator.
func InitSimulator(ctx context.Context) error {
	ports, err := serial.GetPortsList()
	if err != nil {
		return errors.Wrap(err, "failed to get port list")
	}

	if len(ports) == 0 {
		return errors.New("no serial ports found")
	}
	// Open the first serial port detected at 9600bps N81.
	mode := &serial.Mode{
		BaudRate: 9600,
	}
	// Print the list of detected ports.
	for _, port := range ports {
		usbPort, err := serial.Open(port, mode)
		if err != nil {
			return errors.Wrap(err, "serial open error")
		}

		var t = 3 * time.Second
		usbPort.SetReadTimeout(t)
		checkData := []byte{0x57, 0xAB}
		_, err = usbPort.Write([]byte(MediaRelease))
		if err != nil {
			return errors.Wrap(err, "serial write error")
		}
		_, err = usbPort.Write([]byte("\n"))
		if err != nil {
			return errors.Wrap(err, "serial write newline error")
		}

		buff := make([]byte, 1000)
		for {
			// Reads up to 1000 bytes.
			n, err := usbPort.Read(buff)

			if err != nil {
				return errors.Wrap(err, "serial read error")
			}
			if n == 0 {
				break
			}
			if bytes.Contains(buff, checkData) {
				simulatorOnline = port
				testing.ContextLogf(ctx, "Simulator: %s", port)
				break
			}
		}
		usbPort.Close()
	}
	if simulatorOnline != "" {
		simulatorSerial, err = serial.Open(simulatorOnline, mode)
		if err != nil {
			return errors.Wrap(err, "failed to open simulator")
		}
		// Pressing a character key resolves the issue of the keyboard settings disappearing,
		// but it may result in some key functions not performing as expected. The bug link is provided below.
		// https://partnerissuetracker.corp.google.com/u/0/issues/296981810
		// https://partnerissuetracker.corp.google.com/issues/255684001
		if err := SinglePressKeyboard(ctx, ChatKeyCodes["G"].KeyCodeBytes, 100*time.Millisecond); err != nil {
			return errors.Wrap(err, "failed to press 'g'")
		}
	} else {
		return errors.New("failed to find simulator")
	}

	return nil
}

// ReleaseSimulator releases all keys and close simulator.
func ReleaseSimulator(ctx context.Context) error {
	if simulatorSerial == nil {
		return errors.New("simulator is nil")
	}
	if err := executeSimulator(MouseRelease); err != nil {
		return errors.Wrap(err, "failed to release mouse")
	}
	if err := executeSimulator(KeyboardRelease); err != nil {
		return errors.Wrap(err, "failed to release keyboard")
	}
	if err := executeSimulator(MediaRelease); err != nil {
		return errors.Wrap(err, "failed to release media key")
	}
	if err := simulatorSerial.Close(); err != nil {
		return errors.Wrap(err, "failed to close simulator")
	}
	testing.ContextLog(ctx, "close simulator")
	simulatorOnline = ""
	simulatorSerial = nil
	return nil
}

// CombineKeyPressKeyboard is used to input combination keys,
// allowing input for up to six keys simultaneously, with the capability to set the duration.
func CombineKeyPressKeyboard(ctx context.Context, duration time.Duration, keyCodes []int) error {
	kc := KeyboardBase
	for index, key := range keyCodes {
		kc[index+7] = byte(key)
	}
	key := calculationVerificationCode(kc)
	if err := SinglePressKeyboard(ctx, key, duration); err != nil {
		return errors.Wrap(err, "failed to click combine key")
	}

	return nil
}

// SinglePressKeyboard is used to input single key, with the capability to set the duration.
func SinglePressKeyboard(ctx context.Context, keyCode []byte, duration time.Duration) error {
	if err := executeSimulator(keyCode); err != nil {
		return errors.Wrapf(err, "failed to click key code:%v", keyCode)
	}
	// GoBigSleepLint: press time.
	testing.Sleep(ctx, duration)
	if err := executeSimulator(KeyboardRelease); err != nil {
		return errors.Wrap(err, "failed to release all keyboard keys")
	}
	// GoBigSleepLint: avoid executing too quickly to prevent incomplete key release.
	testing.Sleep(ctx, 200*time.Millisecond)
	if err := executeSimulator(MediaRelease); err != nil {
		return errors.Wrap(err, "failed to release media key")
	}
	return nil
}

// MouseClick is used to input mouse key, with the capability to set the duration.
func MouseClick(ctx context.Context, keyCode []byte, duration time.Duration) error {
	testing.ContextLogf(ctx, "mouse click key:%v", keyCode)
	if err := executeSimulator(keyCode); err != nil {
		return errors.Wrapf(err, "failed to click mouse key code:%s", keyCode)
	}
	// GoBigSleepLint: press time.
	testing.Sleep(ctx, duration)
	if err := executeSimulator(MouseRelease); err != nil {
		return errors.Wrap(err, "failed to release mouse key")
	}
	return nil
}

// MouseScroll is used to simulate mouse scrolling, allowing a maximum of 127 movements.
// Positive numbers represent scrolling up, while negative numbers represent scrolling down.
func MouseScroll(ctx context.Context, times int) error {
	kc := MouseBase
	scrollTimes, err := convertingPositiveAndNegativeValues(times)
	if err != nil {
		return errors.Wrap(err, "failed to converting times")
	}
	kc[9] = byte(scrollTimes)
	key := calculationVerificationCode(kc)
	testing.ContextLogf(ctx, "mouse scroll times:%v", times)
	if err := executeSimulator(key); err != nil {
		return errors.Wrapf(err, "failed to scroll mouse times:%v", times)
	}
	return nil
}

// MouseMove is used to simulate mouse move, allowing a maximum of 127 movements.
// Positive 'x' indicates movement to the right, while negative signifies movement to the left.
// Positive 'y' indicates movement to the down, while negative signifies movement to the up.
func MouseMove(ctx context.Context, x, y int) error {
	kc := MouseBase
	xPixel, err := convertingPositiveAndNegativeValues(x)
	if err != nil {
		return errors.Wrap(err, "failed to converting x")
	}
	yPixel, err := convertingPositiveAndNegativeValues(y)
	if err != nil {
		return errors.Wrap(err, "failed to converting y")
	}
	kc[7] = byte(xPixel)
	kc[8] = byte(yPixel)
	key := calculationVerificationCode(kc)
	testing.ContextLogf(ctx, "mouse move x:%v y:%v", x, y)
	if err := executeSimulator(key); err != nil {
		return errors.Wrap(err, "failed to mouse move")
	}
	return nil
}
func convertingPositiveAndNegativeValues(value int) (int, error) {
	if value >= 0 && value < 128 {
		return value, nil
	} else if value > -128 && value < 0 {
		return value + 256, nil
	} else {
		return 0, errors.New("the value must range from 0 to 127 or from -1 to -127")
	}
}
func calculationVerificationCode(base []byte) []byte {
	sum := 0
	for _, b := range base {
		sum = sum + int(b)
	}
	sum = sum % 256
	base = append(base, byte(sum))
	return base
}
func executeSimulator(keyCode []byte) error {
	if simulatorSerial == nil {
		return errors.New("simulator is nil")
	}
	if _, err := simulatorSerial.Write(keyCode); err != nil {
		return errors.Wrap(err, "failed to write keycode to simulator")
	}
	if _, err := simulatorSerial.Write([]byte("\n")); err != nil {
		return errors.Wrap(err, "failed to write newline to simulator")
	}
	return nil
}
