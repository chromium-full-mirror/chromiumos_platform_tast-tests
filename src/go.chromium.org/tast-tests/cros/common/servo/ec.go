// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// These are the EC Servo controls which can be get/set with a string value.
const (
	ECBoard            StringControl = "ec_board"
	ECSystemPowerState StringControl = "ec_system_powerstate"
	ECUARTCmd          StringControl = "ec_uart_cmd"
	ECUARTRegexp       StringControl = "ec_uart_regexp"
	ECUARTStream       StringControl = "ec_uart_stream"
	ECChip             StringControl = "ec_chip"
	ECFlashSize        StringControl = "ec_flash_size"
	DUTPDDataRole      StringControl = "dut_pd_data_role"
)

// These controls accept only "on" and "off" as values.
const (
	ECUARTCapture   OnOffControl = "ec_uart_capture"
	ECUARTTimestamp OnOffControl = "ec_uart_timestamp"
)

// Cmd constants for RunECCommand.
const (
	// Using with no additional arguments returns current backlight level
	// If additional int arg (0-100) provided, sets backlight to that level
	kbLight string = "kblight"
)

// Pattern expression for RunCommandGetOutput.
const (
	reKBBacklight        string = `Keyboard backlight: (\d+)\%`
	reCheckKBLight       string = `Keyboard backlight: \d+\%|Command 'kblight' not found or ambiguous.`
	reTabletmodeNotFound string = `Command 'tabletmode' not found or ambiguous`
	reBasestateNotFound  string = `Command 'basestate' not found or ambiguous`
	reTabletmodeStatus   string = `\[\S+ tablet mode (enabled|disabled)\]`
	reBasestateStatus    string = `\[\S+ base state: (attached|detached)\]`
	reBdStatus           string = `\[\S+ BD forced (connected|disconnected)\]`
	reLidAccel           string = `\[\S+ Lid Accel ODR:(?i)[^\n\r]*(?i)(1|0)\S+]`
	reVupBtnPressed      string = `\[\S+ Button \'Volume Up\' was pressed(.|\n)*buttons: 2\]`
	reVupBtnReleased     string = `\[\S+ Button \'Volume Up\' was released(.|\n)*buttons: 0\]`
	reVdownBtnPressed    string = `\[\S+ Button \'Volume Down\' was pressed(.|\n)*buttons: 4\]`
	reVdownBtnReleased   string = `\[\S+ Button \'Volume Down\' was released(.|\n)*buttons: 0\]`
	rePwrBtnPressed      string = `\[\S+ power button pressed(.|\n)*buttons: 1\]`
	rePwrBtnReleased     string = `\[\S+ power button released(.|\n)*buttons: 0\]`
)

// USBCDataRole is a USB-C data role.
type USBCDataRole string

// USB-C data roles.
const (
	// UFP is Upward facing partner, i.e. a peripheral. The servo should normally be in this role.
	UFP USBCDataRole = "UFP"
	// DFP is Downward facing partner, i.e. a host. The DUT should normally be in this role.
	DFP USBCDataRole = "DFP"
)

// KBMatrixPair is a struct to store key row and col for the kbpress cmd
type KBMatrixPair struct {
	row int
	col int
}

// KeyMatrix is a map that stores a row/col pair for each key using KBMatrixPair
// It's stored in order of appearance in a keyboard
var KeyMatrix = map[string]KBMatrixPair{
	"<esc>":       KBMatrixPair{1, 1},
	"<f1>":        KBMatrixPair{0, 2},
	"<f2>":        KBMatrixPair{3, 2},
	"<f3>":        KBMatrixPair{2, 2},
	"<f4>":        KBMatrixPair{1, 2},
	"<f5>":        KBMatrixPair{3, 4},
	"<f6>":        KBMatrixPair{2, 4},
	"<f7>":        KBMatrixPair{1, 4},
	"<f8>":        KBMatrixPair{2, 9},
	"<f9>":        KBMatrixPair{1, 9},
	"<f10>":       KBMatrixPair{0, 4},
	"`":           KBMatrixPair{3, 1},
	"1":           KBMatrixPair{6, 1},
	"2":           KBMatrixPair{6, 4},
	"3":           KBMatrixPair{6, 2},
	"4":           KBMatrixPair{6, 3},
	"5":           KBMatrixPair{3, 3},
	"6":           KBMatrixPair{3, 6},
	"7":           KBMatrixPair{6, 6},
	"8":           KBMatrixPair{6, 5},
	"9":           KBMatrixPair{6, 9},
	"0":           KBMatrixPair{6, 8},
	"-":           KBMatrixPair{3, 8},
	"=":           KBMatrixPair{0, 8},
	"<backspace>": KBMatrixPair{1, 11},
	"<tab>":       KBMatrixPair{2, 1},
	"q":           KBMatrixPair{7, 1},
	"w":           KBMatrixPair{7, 4},
	"e":           KBMatrixPair{7, 2},
	"r":           KBMatrixPair{7, 3},
	"t":           KBMatrixPair{2, 3},
	"y":           KBMatrixPair{2, 6},
	"u":           KBMatrixPair{7, 6},
	"i":           KBMatrixPair{7, 5},
	"o":           KBMatrixPair{7, 9},
	"p":           KBMatrixPair{7, 8},
	"[":           KBMatrixPair{2, 8},
	"]":           KBMatrixPair{2, 5},
	"\\":          KBMatrixPair{3, 11},
	"<search>":    KBMatrixPair{0, 1},
	"a":           KBMatrixPair{4, 1},
	"s":           KBMatrixPair{4, 4},
	"d":           KBMatrixPair{4, 2},
	"f":           KBMatrixPair{4, 3},
	"g":           KBMatrixPair{1, 3},
	"h":           KBMatrixPair{1, 6},
	"j":           KBMatrixPair{4, 6},
	"k":           KBMatrixPair{4, 5},
	"l":           KBMatrixPair{4, 9},
	";":           KBMatrixPair{4, 8},
	"'":           KBMatrixPair{1, 8},
	"<enter>":     KBMatrixPair{4, 11},
	"<shift_l>":   KBMatrixPair{5, 7},
	"z":           KBMatrixPair{5, 1},
	"x":           KBMatrixPair{5, 4},
	"c":           KBMatrixPair{5, 2},
	"v":           KBMatrixPair{5, 3},
	"b":           KBMatrixPair{0, 3},
	"n":           KBMatrixPair{0, 6},
	"m":           KBMatrixPair{5, 6},
	",":           KBMatrixPair{5, 5},
	".":           KBMatrixPair{5, 9},
	"/":           KBMatrixPair{5, 8},
	"<shift_r>":   KBMatrixPair{7, 7},
	"<ctrl_l>":    KBMatrixPair{2, 0},
	"<alt_l>":     KBMatrixPair{6, 10},
	" ":           KBMatrixPair{5, 11},
	"<alt_r>":     KBMatrixPair{0, 10},
	"<ctrl_r>":    KBMatrixPair{4, 0},
	"<left>":      KBMatrixPair{7, 12},
	"<up>":        KBMatrixPair{7, 11},
	"<down>":      KBMatrixPair{6, 11},
	"<right>":     KBMatrixPair{6, 12},
}

// HibernationOpt is an option for hibernating DUT.
type HibernationOpt string

// Available options for triggering hibernation.
const (
	// UseKeyboard uses keyboard shortcut for hibernating DUT: alt+vol_up+h.
	UseKeyboard HibernationOpt = "keyboard"
	// UseConsole uses the EC command `hibernate` to put DUT in hibernation.
	UseConsole HibernationOpt = "console"
)

// USBPdDualRoleValue contains a gettable/settable string accepted by
// the ec command, 'pd <port> dualrole'.
type USBPdDualRoleValue string

// These are acceptable states for the USB PD dual-role.
const (
	USBPdDualRoleOn     USBPdDualRoleValue = "on"
	USBPdDualRoleOff    USBPdDualRoleValue = "off"
	USBPdDualRoleFreeze USBPdDualRoleValue = "freeze"
	USBPdDualRoleSink   USBPdDualRoleValue = "force sink"
	USBPdDualRoleSource USBPdDualRoleValue = "force source"
)

// RunECCommand runs the given command on the EC on the device.
func (s *Servo) RunECCommand(ctx context.Context, cmd string) error {
	if err := s.SetString(ctx, ECUARTRegexp, "None"); err != nil {
		return errors.Wrap(err, "Clearing EC UART Regexp")
	}
	return s.SetString(ctx, ECUARTCmd, cmd)
}

// RunECCommandGetOutput runs the given command on the EC on the device and returns the output matching patterns.
// It is recommended to send "chan save", "chan 0" just before and "chan restore" afterwards to prevent other logging from interrupting your command.
func (s *Servo) RunECCommandGetOutput(ctx context.Context, cmd string, patterns []string) ([][]string, error) {
	err := s.SetStringList(ctx, ECUARTRegexp, patterns)
	if err != nil {
		return nil, errors.Wrapf(err, "setting ECUARTRegexp to %s", patterns)
	}
	defer s.SetString(ctx, ECUARTRegexp, "None")
	err = s.SetString(ctx, ECUARTCmd, cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "setting ECUARTCmd to %s", cmd)
	}
	iList, err := s.GetStringList(ctx, ECUARTCmd)
	if err != nil {
		return nil, errors.Wrap(err, "decoding string list")
	}
	return ConvertToStringArrayArray(ctx, iList)
}

// GetECSystemPowerState returns the power state, like "S0" or "G3"
func (s *Servo) GetECSystemPowerState(ctx context.Context) (string, error) {
	return s.GetString(ctx, ECSystemPowerState)
}

// ECHibernate puts the EC into hibernation mode, after removing the servo watchdogs.
func (s *Servo) ECHibernate(ctx context.Context, option HibernationOpt) error {
	if err := s.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "failed to remove watchdog for ccd")
	}
	switch option {
	case "keyboard":
		if err := func(ctx context.Context) error {
			for _, targetKey := range []string{"<alt_l>", "<f10>", "h"} {
				row, col, err := s.GetKeyRowCol(targetKey)
				if err != nil {
					return errors.Wrapf(err, "failed to get key %s column and row", targetKey)
				}
				targetKeyName := targetKey
				targetKeyHold := fmt.Sprintf("kbpress %d %d 1", col, row)
				targetKeyRelease := fmt.Sprintf("kbpress %d %d 0", col, row)
				testing.ContextLogf(ctx, "Pressing and holding key %s", targetKey)
				if err := s.RunECCommand(ctx, targetKeyHold); err != nil {
					return errors.Wrapf(err, "failed to press and hold key %s", targetKey)
				}
				defer func(releaseKey, name string) error {
					testing.ContextLogf(ctx, "Releasing key %s", name)
					if err := s.RunECCommand(ctx, releaseKey); err != nil {
						return errors.Wrapf(err, "failed to release key %s", releaseKey)
					}
					return nil
				}(targetKeyRelease, targetKeyName)
			}
			return nil
		}(ctx); err != nil {
			return err
		}
	case "console":
		if err := s.RunECCommand(ctx, "hibernate"); err != nil {
			return errors.Wrap(err, "failed to run EC command: hibernate")
		}
	}

	// Delay for a few seconds to allow proper propagation of the
	// hibernation command, prior to checking EC unresponsive.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	if err := s.CheckUnresponsiveEC(ctx); err != nil {
		return errors.Wrap(err, "while verifying whether EC is unresponsive after hibernating DUT")
	}
	return nil
}

// GetECFlashSize returns the size of EC in KB e.g. 512
func (s *Servo) GetECFlashSize(ctx context.Context) (int, error) {
	sizeStr, err := s.GetString(ctx, ECFlashSize)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get value for ec size")
	}
	// ECFlashSize method matches an int regex so Atoi should always work
	return strconv.Atoi(sizeStr)
}

// GetECChip returns the DUT chip e.g. "npcx_uut"
func (s *Servo) GetECChip(ctx context.Context) (string, error) {
	return s.GetString(ctx, ECChip)
}

// SetDUTPDDataRole tries to find the port attached to the servo, and performs a data role swap if the role doesn't match `role`.
// Will fail if there is no chromeos EC.
func (s *Servo) SetDUTPDDataRole(ctx context.Context, role USBCDataRole) error {
	return s.SetString(ctx, DUTPDDataRole, string(role))
}

// GetKeyRowCol returns the key row and column for kbpress cmd
func (s *Servo) GetKeyRowCol(key string) (int, int, error) {
	pair, ok := KeyMatrix[key]
	if !ok {
		return 0, 0, errors.New("failed to find key in KeyMatrix map")
	}
	return pair.row, pair.col, nil

}

// ECPressKey simulates a keypress on the DUT from the servo using kbpress.
func (s *Servo) ECPressKey(ctx context.Context, key string) error {
	row, col, err := s.GetKeyRowCol(key)
	if err != nil {
		return errors.Wrapf(err, "failed to get key %q in key matrix", key)
	}
	if err := s.RunECCommand(ctx, fmt.Sprintf("kbpress %d %d 1", col, row)); err != nil {
		return errors.Wrapf(err, "failed to press key %q", key)
	}
	if err := s.RunECCommand(ctx, fmt.Sprintf("kbpress %d %d 0", col, row)); err != nil {
		return errors.Wrapf(err, "failed to release key %q", key)
	}
	return nil
}

// SetKBBacklight sets the DUT keyboards backlight to the given value (0 - 100).
func (s *Servo) SetKBBacklight(ctx context.Context, percent int) error {
	testing.ContextLog(ctx, "Setting keyboard backlight to: ", percent)
	err := s.RunECCommand(ctx, fmt.Sprintf("%v %v", kbLight, percent))
	if err != nil {
		return errors.Wrapf(err, "running '%v %v' on DUT", kbLight, percent)
	}
	return nil
}

// GetKBBacklight gets the DUT keyboards backlight value in percent (0 - 100).
func (s *Servo) GetKBBacklight(ctx context.Context) (int, error) {
	testing.ContextLog(ctx, "Getting current keyboard backlight percent")
	out, err := s.RunECCommandGetOutput(ctx, kbLight, []string{reKBBacklight})
	if err != nil {
		return 0, errors.Wrapf(err, "running %v on DUT", kbLight)
	}
	return strconv.Atoi(out[0][1])
}

// HasKBBacklight checks if the DUT keyboards has backlight functionality.
func (s *Servo) HasKBBacklight(ctx context.Context) bool {
	testing.ContextLog(ctx, "Checking if DUT keyboard supports backlight")
	out, _ := s.RunECCommandGetOutput(ctx, kbLight, []string{reCheckKBLight})
	expMatch := regexp.MustCompile(reKBBacklight)
	match := expMatch.FindStringSubmatch(out[0][0])
	return match != nil
}

// CheckUnresponsiveEC verifies that EC console is unresponsive in situations such as
// hibernation and battery cutoff. Ignore null chars, sometimes the servo returns null
// when the EC is off.
func (s *Servo) CheckUnresponsiveEC(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := s.RunECCommandGetOutput(ctx, "version", []string{`[^\x00]+`})
		if err == nil {
			return errors.Errorf("EC is still active: got %v; expected error", out)
		}
		if !strings.Contains(err.Error(), "No data was sent from the pty") &&
			!strings.Contains(err.Error(), "EC: Timeout waiting for response.") &&
			!strings.Contains(err.Error(), "Timed out waiting for interfaces to become available") &&
			!strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") {
			return errors.Wrap(err, "unexpected EC error")
		}
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 1 * time.Minute})
}

// CheckAndRunTabletModeCommand checks if relevant EC commands exist and use them for setting tablet mode.
// For example, detachables use 'basestate (attach|detach)', and convertibles use 'tabletmode (on|off)'.
func (s *Servo) CheckAndRunTabletModeCommand(ctx context.Context, command string) (string, error) {
	// regular expressions.
	reStr := strings.Join([]string{reTabletmodeNotFound, reTabletmodeStatus,
		reBasestateNotFound, reBasestateStatus, reBdStatus, reLidAccel}, "|")
	checkTabletMode := fmt.Sprintf("%s%s%s", "(", reStr, ")")
	// Run EC command to check tablet mode setting.
	out, err := s.RunECCommandGetOutput(ctx, command, []string{checkTabletMode})
	if err != nil {
		return "", errors.Wrapf(err, "failed to run command %q", command)
	}
	tabletModeUnavailable := []*regexp.Regexp{regexp.MustCompile(reTabletmodeNotFound),
		regexp.MustCompile(reBasestateNotFound)}
	for _, v := range tabletModeUnavailable {
		if match := v.FindStringSubmatch(out[0][0]); match != nil {
			return "", errors.Errorf("device does not support tablet mode: %q", match)
		}
	}
	return string(out[0][1]), nil
}

// OpenCCD checks if a CCD connection exists, and then opens CCD if it's locked.
func (s *Servo) OpenCCD(ctx context.Context) error {
	if hasCCD, err := s.HasCCD(ctx); err != nil {
		return errors.Wrap(err, "while checking if servo has a CCD connection")
	} else if hasCCD {
		if val, err := s.GetString(ctx, GSCCCDLevel); err != nil {
			return errors.Wrap(err, "failed to get gsc_ccd_level")
		} else if val != Open {
			testing.ContextLogf(ctx, "CCD is not open, got %q. Attempting to unlock", val)
			if err := s.SetString(ctx, CR50Testlab, Open); err != nil {
				return errors.Wrap(err, "failed to unlock CCD")
			}
		}
		// For debugging purposes, log CCD state after unlocking CCD.
		checkedVal, err := s.GetString(ctx, GSCCCDLevel)
		if err != nil {
			return errors.Wrap(err, "failed to get gsc_ccd_level after unlocking CCD")
		}
		testing.ContextLogf(ctx, "CCD State: %q", checkedVal)
	}
	return nil
}

// CheckUSBPdStatus accepts a port ID and checks for the pd status of this port.
func (s *Servo) CheckUSBPdStatus(ctx context.Context, portID int, expectedStatus USBPdDualRoleValue) error {
	// Some DUTs use a different format in checking for pd: 'pd dualrole'.
	// This format was found on a few models, such as eve, nautilus, soraka,
	// kench, teemo, and sion. Check for the format that works. This list can be
	// further expanded using new test results.
	possibleCmds := []string{fmt.Sprintf("pd %d dualrole", portID), "pd dualrole"}
	matchList := []string{`dual-role toggling: ([^\n\r]*)[\n\r]`}
	checkPdCmd := func() string {
		for _, val := range possibleCmds {
			_, err := s.RunECCommandGetOutput(ctx, val, matchList)
			if err != nil {
				testing.ContextLog(ctx, err.Error())
				continue
			}
			return val
		}
		return ""
	}
	cmd := checkPdCmd()
	if cmd == "" {
		return errors.New("no command found to check for pd")
	}
	testing.ContextLogf(ctx, "Checking for pd port %d status", portID)
	out, err := s.RunECCommandGetOutput(ctx, cmd, matchList)
	if err != nil {
		return errors.Wrapf(err, "failed to run cmd %s", cmd)
	}
	portStatus := USBPdDualRoleValue(out[0][1])
	if portStatus != expectedStatus {
		failStr := fmt.Sprintf("port %d dual-role: %s, expected: %s", portID, portStatus, expectedStatus)
		return errors.New(failStr)
	}
	return nil
}

// ECHostevent holds int codes for EC hostevents
type ECHostevent int64

// Hostevent codes, copied from ec/include/ec_commands.h.
const (
	// HosteventLidClosed is the event code for lid closed.
	HosteventLidClosed ECHostevent = 0x00000001
	// HosteventLidOpen is the event code for lid open.
	HosteventLidOpen ECHostevent = 0x00000002
	// HosteventPowerButton is the event code for power button.
	HosteventPowerButton ECHostevent = 0x00000004
	// HosteventAcConnected is the event code for ac connected.
	HosteventAcConnected ECHostevent = 0x00000008
	// HosteventAcDisconnected is the event code for ac disconnected.
	HosteventAcDisconnected ECHostevent = 0x00000010
	// HosteventBatteryLow is the event code for low battery.
	HosteventBatteryLow ECHostevent = 0x00000020
	// HosteventBatteryCritical is the event code for critical battery.
	HosteventBatteryCritical ECHostevent = 0x00000040
	// HosteventBattery is the event code for battery.
	HosteventBattery ECHostevent = 0x00000080
	// HosteventThermalThreshold is the event code for thermal threshold.
	HosteventThermalThreshold ECHostevent = 0x00000100
	// HosteventThermalOverload is the event code for thermal overload.
	HosteventThermalOverload ECHostevent = 0x00000200
	// HosteventThermal is the event code for thermal.
	HosteventThermal ECHostevent = 0x00000400
	// HosteventUsbCharger is the event code for usb charger.
	HosteventUsbCharger ECHostevent = 0x00000800
	// HosteventKeyPressed is the event code for key press.
	HosteventKeyPressed ECHostevent = 0x00001000
	// HosteventInterfaceReady is the event code for interface ready.
	HosteventInterfaceReady ECHostevent = 0x00002000
	// HosteventKeyboardRecovery is the event code for keyboard recovery combo has been pressed
	HosteventKeyboardRecovery ECHostevent = 0x00004000
	// HosteventThermalShutdown is the event code for shutdown due to thermal overload.
	HosteventThermalShutdown ECHostevent = 0x00008000
	// HosteventBatteryShutdown is the event code for shutdown due to battery level too low.
	HosteventBatteryShutdown ECHostevent = 0x00010000
	// HosteventInvalid is the event code for invalid host event.
	HosteventInvalid ECHostevent = 0x80000000
)

// SetHostevent sets host event in ec console using `hostevet set` cmd.
func (s *Servo) SetHostevent(ctx context.Context, event ECHostevent) error {
	hosteventCmd := fmt.Sprintf("hostevent set 0x%08x", event)
	testing.ContextLogf(ctx, "Setting hostevent: %q", hosteventCmd)
	if err := s.RunECCommand(ctx, hosteventCmd); err != nil {
		return errors.Wrap(err, "failed to set hostevent")
	}
	return nil
}

// ECChannelName holds the ec channel names.
type ECChannelName string

// These are some of the ec channel names available.
// To-do: expand when necessary.
const (
	ECChanKeyboard ECChannelName = "keyboard"
	ECChanSwitch   ECChannelName = "switch"
)

// FindECChanMask accepts an ec channel name, and runs ec 'chan' command to look for
// the corresponding mask value.
func (s *Servo) FindECChanMask(ctx context.Context, chanName ECChannelName) (maskVal string, retErr error) {
	if err := s.RunECCommand(ctx, "chan save"); err != nil {
		return "", errors.Wrap(err, "failed to send 'chan save' to EC")
	}
	if err := s.RunECCommand(ctx, "chan 0"); err != nil {
		return "", errors.Wrap(err, "failed to send 'chan 0' to EC")
	}
	defer func() {
		testing.ContextLog(ctx, "Restoring chan")
		if err := s.RunECCommand(ctx, "chan restore"); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to send 'chan restore' to EC")
			} else {
				testing.ContextLog(ctx, "Failed to send 'chan restore' to EC: ", err)
			}
		}
	}()
	match := fmt.Sprintf(`([0-9a-fA-F]{8})\s+\W?\s+%s`, string(chanName))
	out, err := s.RunECCommandGetOutput(ctx, "chan", []string{match})
	if err != nil {
		return "", err
	}
	if out == nil || len(out[0]) < 2 {
		return "", errors.Errorf("failed to parse chan output correctly, got: %v", out)
	}
	return out[0][1], nil
}

// SetECChanMasks accepts a map of ec channel names with their masks, and sets them.
func (s *Servo) SetECChanMasks(ctx context.Context, ecChanMasks map[ECChannelName]string) error {
	var maskFinal int64
	for name, mask := range ecChanMasks {
		decimalVal, err := strconv.ParseInt(mask, 16, 64)
		if err != nil {
			return errors.Errorf("failed to parse mask value: %s, for ec chan: %s", mask, name)
		}
		maskFinal += decimalVal
	}
	testing.ContextLogf(ctx, "Setting chan mask: %d", maskFinal)
	if err := s.RunECCommand(ctx, fmt.Sprintf("chan %d", maskFinal)); err != nil {
		return errors.Wrap(err, "setting chan mask failed")
	}
	return nil
}

// DetachableECButton holds ec button controls for a detachable,
// which can take customized durations in milliseconds.
type DetachableECButton string

// These are the available ec button controls for a detachable.
const (
	ECVupButton   DetachableECButton = "button vup"
	ECVdownButton DetachableECButton = "button vdown"
	ECPwrButton   DetachableECButton = "powerbtn"
)

// PressECBtnVerifyOutput sends a DetachableECButton and verifies in the output that
// the button was successfully pressed and released. Call FindECChanMask first to find
// the mask values for ECChanKeyboard and ECChanSwitch, and pass them to PressECBtnVerifyOutput.
func (s *Servo) PressECBtnVerifyOutput(ctx context.Context, button DetachableECButton, duration int, ecChanMasks map[ECChannelName]string) error {
	requiredMasks := []ECChannelName{ECChanKeyboard, ECChanSwitch}
	for _, name := range requiredMasks {
		if _, ok := ecChanMasks[name]; !ok {
			return errors.Errorf("missing mask value for ec chan: %s", name)
		}
	}
	if err := s.SetECChanMasks(ctx, ecChanMasks); err != nil {
		return err
	}
	var checkPressEffective string
	switch button {
	case ECVupButton:
		checkPressEffective = `(` + reVupBtnPressed + `(.|\n)*` + reVupBtnReleased + `)`
	case ECVdownButton:
		checkPressEffective = `(` + reVdownBtnPressed + `(.|\n)*` + reVdownBtnReleased + `)`
	case ECPwrButton:
		checkPressEffective = `(` + rePwrBtnPressed + `(.|\n)*` + rePwrBtnReleased + `)`
	default:
		return errors.Errorf("unable to recognize %s", button)
	}
	testing.ContextLogf(ctx, "Pressing %s for %d milliseconds", button, duration)
	control := fmt.Sprintf("%s %s", button, strconv.Itoa(duration))
	if _, err := s.RunECCommandGetOutput(ctx, control, []string{checkPressEffective}); err != nil {
		return errors.Wrapf(err, "pressing %s failed", button)
	}
	return nil
}
