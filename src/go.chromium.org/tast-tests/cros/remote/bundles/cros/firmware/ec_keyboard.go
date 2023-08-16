// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type keyboardTest int

const (
	servoUSBKeyboard keyboardTest = iota
	servoECKeyboard
	convertibleKeyboard
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECKeyboard,
		Desc: "Test EC Keyboard interface",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Keyboard()),
		Requirements: []string{"sys-fw-0022-v02"},
		Fixture:      fixture.NormalMode,
		Timeout:      2 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		Params: []testing.Param{{
			Val:               servoECKeyboard,
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Detachable, hwdep.Convertible)),
			ExtraAttr:         []string{"firmware_ec"},
		}, {
			Name:      "usb_keyboard",
			Val:       servoUSBKeyboard,
			ExtraAttr: []string{"firmware_ec"},
		}, {
			Name:              "convertible",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible)),
			Val:               convertibleKeyboard,
			ExtraAttr:         []string{"firmware_ec"},
		}},
	})
}

const (
	typeTimeout = 500 * time.Millisecond
	keyPressDur = 100 * time.Millisecond // Equivalent to DurTab keypress.
)

func ECKeyboard(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Requiring RPC utils: ", err)
	}

	testType := s.Param().(keyboardTest)
	// Make sure internal keyboard is connected for convertible devices.
	// Isn't always required so attempt test anyway if this fails.
	if testType == convertibleKeyboard {
		if _, err := h.Servo.CheckAndRunTabletModeCommand(ctx, "tabletmode off"); err != nil {
			s.Log("Failed to set tabletmode to off: ", err)
		}
	}

	// Stop UI to prevent keypresses from causing unintended behaviour.
	if out, err := h.DUT.Conn().CommandContext(ctx, "status", "ui").Output(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to check ui status: ", err)
	} else if !strings.Contains(string(out), "stop/waiting") {
		s.Log("Stopping UI")
		if err := h.DUT.Conn().CommandContext(ctx, "stop", "ui").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to stop ui: ", err)
		}
	}
	defer func() {
		// Restart UI after test ends.
		if err := h.DUT.Conn().CommandContext(ctx, "start", "ui").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to start ui: ", err)
		}
	}()

	var device string
	var testKeyMap map[string]string
	var keyPressFunc func(context.Context, string) error

	switch testType {
	case servoUSBKeyboard:
		if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.On); err != nil {
			s.Fatal("Failed to enable usb keyboard: ", err)
		}
		// Only usb_keyboard_enter_key uses the usb keyboard handler in servo.
		testKeyMap = map[string]string{
			"usb_keyboard_enter_key": "KEY_ENTER",
		}
		keyPressFunc = func(ctx context.Context, keyStr string) error {
			key := servo.KeypressControl(keyStr)
			if err := h.Servo.KeypressWithDuration(ctx, key, servo.Dur(keyPressDur)); err != nil {
				return errors.Wrap(err, "failed to type key")
			}
			return nil
		}
		// This is where the usb keyboard device events that servo emulates are sent.
		device = "/dev/input/by-id/usb-Google_Servo_LUFA_Keyboard_Emulator-event-kbd"

	default: // Covers servoECKeyboard, convertibleKeyboard cases.
		if hasKb, err := h.Servo.HasControl(ctx, string(servo.USBKeyboard)); err != nil {
			s.Fatal("Failed to check for usb keyboard: ", err)
		} else if hasKb {
			if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.Off); err != nil {
				s.Fatal("Failed to disable usb keyboard: ", err)
			}
		}
		testKeyMap = map[string]string{
			"0":        "KEY_0",
			"b":        "KEY_B",
			"e":        "KEY_E",
			"o":        "KEY_O",
			"r":        "KEY_R",
			"s":        "KEY_S",
			"t":        "KEY_T",
			"<enter>":  "KEY_ENTER",
			"<ctrl_l>": "KEY_LEFTCTRL",
			"<alt_l>":  "KEY_LEFTALT",
			"<esc>":    "KEY_ESC",
			"<tab>":    "KEY_TAB",
			// Older devices have overview (scale) on F5, but the spec at https://chromeos.google.com/partner/dlm/docs/hardware-specs/keyboardspec.html uses screenshot (sysrq)
			"<f5>": "KEY_F5|KEY_SCALE|KEY_SYSRQ",
			"<f6>": "KEY_F6|KEY_BRIGHTNESSDOWN",
			"<f7>": "KEY_F7|KEY_BRIGHTNESSUP",
			" ":    "KEY_SPACE",
		}
		keyPressFunc = func(ctx context.Context, key string) error {
			if err := h.Servo.PressKey(ctx, key, servo.Dur(keyPressDur)); err != nil {
				return errors.Wrap(err, "failed to type key")
			}
			return nil
		}
		res, err := h.RPCUtils.FindPhysicalKeyboard(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("During FindPhysicalKeyboard: ", err)
		}
		device = res.Path

	}

	s.Log("Device path: ", device)
	cmd := h.DUT.Conn().CommandContext(ctx, "evtest", device)
	stdout, err := cmd.StdoutPipe()
	scanner := bufio.NewScanner(stdout)
	cmd.Start()

	// Read and discard initial info text.
	func() {
		text := make(chan string)
		go func() {
			defer close(text)
			for scanner.Scan() {
				text <- scanner.Text()
			}
		}()
		for {
			select {
			case <-time.After(1 * time.Second):
				// Time out after 1 second so it doesn't get stuck here.
				s.Log("Finshed reading preamble")
				return
			case _ = <-text:
				continue
			}
		}
	}()

	for key, keyCode := range testKeyMap {
		s.Logf("Pressing key %q, expecting to read keycode %q", key, keyCode)
		if err = readKeyPress(ctx, h, scanner, key, keyCode, keyPressFunc); err != nil {
			s.Fatal("Failed to read key: ", err)
		}
		// Wait for reading to complete before entering next key to prevent failing previous read.
		if err = testing.Sleep(ctx, typeTimeout); err != nil {
			s.Fatalf("Failed to sleep for %s waiting to type next key", typeTimeout)
		}
	}
}

func readKeyPress(ctx context.Context, h *firmware.Helper, scanner *bufio.Scanner, key, keyCode string,
	keyPress func(context.Context, string) error) error {
	regex := `Event.*time.*code\s(\d*)\s\(` + keyCode + `\)`
	expMatch := regexp.MustCompile(regex)

	text := make(chan string)
	go func() {
		defer close(text)
		for scanner.Scan() {
			text <- scanner.Text()
		}
	}()

	start := time.Now()
	if err := keyPress(ctx, key); err != nil {
		return errors.Wrap(err, "failed to type key")
	}

	for {
		select {
		case <-time.After(typeTimeout):
			return errors.New("did not detect keycode within expected time")
		case out := <-text:
			if match := expMatch.FindStringSubmatch(out); match != nil {
				testing.ContextLogf(ctx, "key pressed detected in %s: %v", time.Since(start)-keyPressDur, match)
				return nil
			}
		}
	}
}
