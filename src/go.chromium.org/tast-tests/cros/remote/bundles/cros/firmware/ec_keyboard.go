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
	"go.chromium.org/tast-tests/cros/common/tbdep"
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
		Desc: "Verifies that the emulated keyboard works for all important keys",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw", "group:labqual"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Keyboard()),
		Fixture:      fixture.NormalMode,
		Timeout:      5 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		Params: []testing.Param{{
			Val:               servoECKeyboard,
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Detachable, hwdep.Convertible)),
		}, {
			Name: "usb_keyboard",
			Val:  servoUSBKeyboard,
		}, {
			Name:              "convertible",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible)),
			Val:               convertibleKeyboard,
		}},
	})
}

const (
	typeTimeout = 1 * time.Second
	keyPressDur = 100 * time.Millisecond // Equivalent to DurTab keypress.
)

// ECKeyboard tests the emulated keyboard works. If this test fails, you may need to edit the keymapping in src/go.chromium.org/tast-tests/cros/remote/firmware/keyboard.go
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
		if _, err := h.Servo.RunTabletModeCommandGetOutput(ctx, "tabletmode off"); err != nil {
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
	expectedKeyMap := map[string]string{}

	switch testType {
	case servoUSBKeyboard:
		if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.On); err != nil {
			s.Fatal("Failed to enable usb keyboard: ", err)
		}
		// Only usb_keyboard_enter_key uses the usb keyboard handler in servo.
		testKeyMap = map[string]string{
			"KEY_ENTER": "usb_keyboard_enter_key",
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
		if err := h.RequirePlatform(ctx); err != nil {
			s.Error("Could not read platform: ", err)
		}
		var err error
		testKeyMap, expectedKeyMap, err = firmware.GetKeyboardMappings(ctx, h.DUT, h.Model, s.Features("").Hardware.HardwareFeatures)
		if err != nil {
			s.Fatal("Failed to get keyboard mappings: ", err)
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
	stdout, _ := cmd.StdoutPipe()
	scanner := bufio.NewScanner(stdout)
	cmd.Start()
	defer stdout.Close()
	defer cmd.Abort()

	// Read stdout in background
	text := make(chan string)
	go func() {
		defer close(text)
		for scanner.Scan() {
			text <- scanner.Text()
		}
	}()

	// Servo can't press F11+ on most devices, see src/third_party/hdctools/servo/drv/keyboard_handlers.py
	// If one of the important keys is on F11 or F12, edit the keyboard matrix in keyboard_handlers.py.
	for keyCode, key := range testKeyMap {
		if alt, ok := expectedKeyMap[key]; ok {
			keyCode = alt
		}
		s.Logf("Pressing key %q, expecting to read keycode %q", key, keyCode)
		if err := readKeyPress(ctx, h, text, key, keyCode, keyPressFunc); err != nil {
			s.Errorf("Failed to read key %q: %v", keyCode, err)
		}
	}
}

func readKeyPress(ctx context.Context, h *firmware.Helper, text chan string, key, keyCode string,
	keyPress func(context.Context, string) error) error {
	// Event: time 1707783159.027244, type 1 (EV_KEY), code 15 (KEY_TAB), value 0
	regex := `Event.*time.*code\s(\d*)\s\((KEY_[^\)]*)\).* value 0`
	expMatch := regexp.MustCompile(regex)

	start := time.Now()
	if err := keyPress(ctx, key); err != nil {
		return errors.Wrap(err, "failed to type key")
	}

	for {
		select {
		case <-time.After(typeTimeout):
			return errors.New("did not detect keycode within expected time")
		case out := <-text:
			testing.ContextVLogf(ctx, "evtest: %s", out)
			if match := expMatch.FindStringSubmatch(out); match != nil {
				if match[2] == keyCode {
					testing.ContextLogf(ctx, "key pressed detected in %s: %v", time.Since(start)-keyPressDur, match[2])
					return nil
				}
				testing.ContextLogf(ctx, "Unexpected key pressed: %s", match[2])
			}
		}
	}
}
