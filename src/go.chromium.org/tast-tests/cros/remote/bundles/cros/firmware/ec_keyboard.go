// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
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
		Attr:         []string{"group:firmware", "firmware_ec", "group:labqual"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Keyboard()),
		Requirements: []string{"sys-fw-0022-v02"},
		Fixture:      fixture.NormalMode,
		Timeout:      5 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
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
			" ":        "KEY_SPACE",
		}
		// Run ectool to get vivaldi keys (requires OS >= R123-15775.0.0)
		out, err := h.DUT.Conn().CommandContext(ctx, "ectool", "kbgetconfig").CombinedOutput()
		if err != nil {
			// Host command not implemented: EC result 1 (INVALID_COMMAND)
			// Vivaldi keyboard not enabled: EC result 2 (ERROR)
			if strings.Contains(string(out), "EC result 1 (INVALID_COMMAND)") || strings.Contains(string(out), "EC result 2 (ERROR)") {
				// Non-vivaldi devices that have key mappings
				// 13577 trogdor/pazquel360: F5=KEY_SYSRQ F6=KEY_BRIGHTNESSDOWN F7=KEY_BRIGHTNESSUP
				// 13885 asurada/spherion:   F5=KEY_SYSRQ F6=KEY_BRIGHTNESSDOWN F7=KEY_BRIGHTNESSUP
				// 14454 cherry/tomato:      F5=KEY_SYSRQ F6=KEY_BRIGHTNESSDOWN F7=KEY_BRIGHTNESSUP
				// 15194 corsola/steelix:    F5=KEY_BRIGHTNESSDOWN F6=KEY_BRIGHTNESSUP F7=KEY_MICMUTE
				if err := h.RequirePlatform(ctx); err != nil {
					s.Error("Could not read platform: ", err)
				}
				if h.Model == "steelix" {
					s.Log("Testing steelix keys: KEY_BRIGHTNESSDOWN, KEY_BRIGHTNESSUP, KEY_MICMUTE")
					testKeyMap["<f5>"] = "KEY_BRIGHTNESSDOWN"
					testKeyMap["<f6>"] = "KEY_BRIGHTNESSUP"
					testKeyMap["<f7>"] = "KEY_MICMUTE"
				} else if h.Model == "hayato" {
					s.Log("Testing non-vivaldi keys: KEY_SCALE, KEY_RIGHTNESSDOWN, KEY_BRIGHTNESSUP")
					testKeyMap["<f5>"] = "KEY_SCALE"
					testKeyMap["<f6>"] = "KEY_BRIGHTNESSDOWN"
					testKeyMap["<f7>"] = "KEY_BRIGHTNESSUP"
				} else if s.Features("").Hardware.HardwareFeatures.FwConfig.FwRoVersion.MajorVersion >= 13885 || h.Model == "pompom" || h.Model == "kingoftown" || h.Model == "pazquel" || h.Model == "pazquel360" {
					s.Log("Testing non-vivaldi keys: KEY_SYSRQ, KEY_BRIGHTNESSDOWN, KEY_BRIGHTNESSUP")
					testKeyMap["<f5>"] = "KEY_SYSRQ"
					testKeyMap["<f6>"] = "KEY_BRIGHTNESSDOWN"
					testKeyMap["<f7>"] = "KEY_BRIGHTNESSUP"
				} else {
					// This is a legacy device
					// 13577 trogdor/lazor
					s.Log("Testing legacy keys: F5, F6, F7")
					testKeyMap["<f5>"] = "KEY_F5"
					testKeyMap["<f6>"] = "KEY_F6"
					testKeyMap["<f7>"] = "KEY_F7"
				}
			} else {
				s.Fatalf("ectool kbgetconfig failed: %v, %s", err, string(out))
			}
		} else {
			// Servo can't press F11+, see src/third_party/hdctools/servo/drv/keyboard_handlers.py
			s.Log("Testing vivaldi keys F1-F10")
			sc := bufio.NewScanner(bytes.NewReader(out))
			re := regexp.MustCompile(`^\s*(\d+): (\S[^\(]*) \(`)
			for sc.Scan() {
				m := re.FindStringSubmatch(sc.Text())
				if m != nil {
					idx, err := strconv.Atoi(m[1])
					if err != nil {
						s.Fatalf("Failed to parse %q in %q", m[1], sc.Text())
					}
					key := fmt.Sprintf("<f%d>", idx+1)
					if idx >= 10 {
						s.Logf("%s: %s IGNORED", key, m[2])
						continue
					}

					// From src/third_party/coreboot/src/acpi/acpigen_ps2_keybd.c
					switch m[2] {
					case "Back":
						testKeyMap[key] = "KEY_BACK"
					case "Refresh":
						testKeyMap[key] = "KEY_REFRESH"
					case "Forward":
						testKeyMap[key] = "KEY_FORWARD"
					case "Fullscreen":
						testKeyMap[key] = "KEY_ZOOM"
					case "Overview":
						testKeyMap[key] = "KEY_SCALE"
					case "Brightness Down":
						testKeyMap[key] = "KEY_BRIGHTNESSDOWN"
					case "Brightness Up":
						testKeyMap[key] = "KEY_BRIGHTNESSUP"
					case "Volume Mute":
						testKeyMap[key] = "KEY_MUTE"
					case "Volume Down":
						testKeyMap[key] = "KEY_VOLUMEDOWN"
					case "Volume Up":
						testKeyMap[key] = "KEY_VOLUMEUP"
					case "Snapshot":
						testKeyMap[key] = "KEY_SYSRQ"
					case "Privacy Screen Toggle":
						testKeyMap[key] = "KEY_PRIVACY_SCREEN_TOGGLE"
					case "Keyboard Backlight Down":
						testKeyMap[key] = "KEY_KBDILLUMDOWN"
					case "Keyboard Backlight Up":
						testKeyMap[key] = "KEY_KBDILLUMUP"
					case "Play/Pause":
						testKeyMap[key] = "KEY_PLAYPAUSE"
					case "Next Track":
						testKeyMap[key] = "KEY_NEXTSONG"
					case "Previous Track":
						testKeyMap[key] = "KEY_PREVIOUSSONG"
					case "Keyboard Backlight Toggle":
						testKeyMap[key] = "KEY_KBDILLUMTOGGLE"
					case "Microphone Mute":
						testKeyMap[key] = "KEY_MICMUTE"
					case "Menu":
						testKeyMap[key] = "KEY_CONTROLPANEL"
					default:
						s.Logf("%s: %s IGNORED", key, m[2])
					}
				}
			}
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

	for key, keyCode := range testKeyMap {
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
