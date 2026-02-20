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
	ECUARTFlush     OnOffControl = "ec_uart_flush"
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
	reCheckKBLight       string = `Keyboard backlight: \d+\%|Command 'kblight' not found or ambiguous`
	reTabletmodeNotFound string = `Command 'tabletmode' not found or ambiguous`
	reBasestateNotFound  string = `Command 'basestate' not found or ambiguous`
	// reTabletmodeStatus prints on channel motionlid
	reTabletmodeStatus string = `\[\S+ tablet mode\s?(enabled|disabled)?|clamshell mode\]`
	// reBasestateStatus prints on channel motionlid
	reBasestateStatus string = `\[\S+ base state: (attached|detached)\]`
	// reBdStatus prints on channel usb or system
	reBdStatus string = `\[\S+ BD forced (connected|disconnected)\]`
	// reBdStatus prints on channel motionsense
	reLidAccel         string = `\[\S+ Lid Accel ODR:[^\n\r]*(1|0)\S+]`
	reVupBtnPressed    string = `\[\S+ Button \'Volume Up\' was pressed(.|\n)*buttons: 2\]`
	reVupBtnReleased   string = `\[\S+ Button \'Volume Up\' was released(.|\n)*buttons: 0\]`
	reVdownBtnPressed  string = `\[\S+ Button \'Volume Down\' was pressed(.|\n)*buttons: 4\]`
	reVdownBtnReleased string = `\[\S+ Button \'Volume Down\' was released(.|\n)*buttons: 0\]`
	rePwrBtnPressed    string = `\[\S+ power button pressed(.|\n)*buttons: 1\]`
	rePwrBtnReleased   string = `\[\S+ power button released(.|\n)*buttons: 0\]`
	reSKUID            string = `SKU_ID:\s+(\d+)`
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
// It is recommended to send "chan save", "chan 0" just before and "chan restore" afterwards to prevent other
// logging from interrupting your command. (RunECCommandGetOutputNoConsoleLogs() can handle this for you)
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

// CaptureECCommand runs the specified command on the EC, capturing all output
// for the specified amount of time.
func (s *Servo) CaptureECCommand(ctx context.Context, cmd string, timeout time.Duration) (retOutput string, retErr error) {
	closeECUART, err := s.EnableUARTCapture(ctx, ECUARTCapture)
	if err != nil {
		return "", errors.Wrap(err, "failed to enable UART capture")
	}
	defer func() {
		retErr = errors.Join(retErr, closeECUART(ctx))
	}()

	ecUartCtx, ecUartCtxCancel := context.WithTimeout(ctx, timeout)
	defer ecUartCtxCancel()
	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()

	// Drain the UART output prior to running the command.
	if _, err := s.GetQuotedString(ctx, ECUARTStream); err != nil {
		return "", errors.Wrap(err, "failed to drain EC UART")
	}

	if err := s.SetString(ctx, ECUARTCmd, cmd); err != nil {
		return "", errors.Wrapf(err, "setting ECUARTCmd to %s", cmd)
	}

	var output string
	var lines string
Loop:
	for {
		select {
		case <-ecUartCtx.Done():
			break Loop
		case <-ticker.C:
			lines, err = s.GetQuotedString(ctx, ECUARTStream)
			if err != nil {
				return "", errors.Wrap(err, "failed to read EC UART")
			}
			output += lines
		}
	}

	return output, nil
}

func (s *Servo) runECCommandGetOutputNoConsoleLogsHelper(ctx context.Context, cmd string, patterns []string, allowRetries bool) (output [][]string, retErr error) {
	// EC console can be extremely chatty. Log messages are liable to interrupt
	// the console output, breaking the regex pattern. Turn off all other channels
	// and restore after.
	if err := s.SaveDUTConsoleChannelMask(ctx); err != nil {
		return nil, errors.Wrap(err, "cannot save current chan mask")
	}

	// 0 turns off all channels except console output, which is always on.
	if err := s.SetDUTConsoleChannelMask(ctx, 0); err != nil {
		return nil, errors.Wrap(err, "cannot disable console logs pre-cmd")
	}

	// Restore original mask
	defer func() {
		if err := s.RestoreDUTConsoleChannelMask(ctx); err != nil {
			retErr = errors.Join(errors.Wrap(err, "cannot restore console chanel mask to original state"))
		}
	}()

	if allowRetries {
		// Use a polling loop to retry the command if not successful.
		var output [][]string
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var errCmd error

			output, errCmd = s.RunECCommandGetOutput(ctx, cmd, patterns)
			if errCmd != nil {
				testing.ContextLogf(ctx, "EC Command %q failed: %q", cmd, errCmd)
			}
			return errCmd
		}, &testing.PollOptions{Timeout: 6 * time.Second, Interval: 2 * time.Second}); err != nil {
			return nil, err
		}
		return output, nil
	}

	// If not allowing retries, just try command once and return
	return s.RunECCommandGetOutput(ctx, cmd, patterns)
}

// RunECCommandGetOutputNoConsoleLogs works like RunECCommandGetOutput but automatically disables
// all console logging, which could interfere with capturing command output.
func (s *Servo) RunECCommandGetOutputNoConsoleLogs(ctx context.Context, cmd string, patterns []string) ([][]string, error) {
	return s.runECCommandGetOutputNoConsoleLogsHelper(ctx, cmd, patterns, false)
}

// RunECCommandGetOutputNoConsoleLogsAllowRetries works like RunECCommandGetOutputNoConsoleLogs but
// allows the command to be retried up to twice if unsuccessful.
func (s *Servo) RunECCommandGetOutputNoConsoleLogsAllowRetries(ctx context.Context, cmd string, patterns []string) ([][]string, error) {
	return s.runECCommandGetOutputNoConsoleLogsHelper(ctx, cmd, patterns, true)
}

// GetECSystemPowerState returns the power state, like "S0" or "G3"
func (s *Servo) GetECSystemPowerState(ctx context.Context) (string, error) {
	return s.GetString(ctx, ECSystemPowerState)
}

// ECHibernate puts the EC into hibernation mode, after removing the servo watchdogs.
func (s *Servo) ECHibernate(ctx context.Context, model string, option HibernationOpt) error {
	if err := s.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "failed to remove watchdog for ccd")
	}
	switch option {
	case "keyboard":
		testing.ContextLog(ctx, "Pressing and holding alt+f10+h")
		if err := s.PressKeys(ctx, []string{"<alt_l>", "<f10>", "h"}, DurTab); err != nil {
			return errors.Wrap(err, "failed to press keys")
		}
	case "console":
		if err := s.RunECCommand(ctx, "hibernate"); err != nil {
			return errors.Wrap(err, "failed to run EC command: hibernate")
		}
	}

	// Delay for a few seconds to allow proper propagation of the
	// hibernation command, prior to checking EC unresponsive.
	// GoBigSleepLint: sleep to let hibernation propagate
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

// TabletModeCmdUnsupportedErr is the error returned by
// RunTabletModeCommandGetOutput when the ec command is found unsupported.
// To detect this error, use something like:
// if _, ok := err.(*TabletModeCmdUnsupportedErr); ok {}
type TabletModeCmdUnsupportedErr struct {
	*errors.E
}

// RunTabletModeCommandGetOutput runs EC commands to set tablet mode and
// returns the output matching pattern for the resulting tablet mode state.
// Before calling RunTabletModeCommand(), a test can call
// h.GetECTabletLaptopModeCommand() to determine the corresponding command.
func (s *Servo) RunTabletModeCommandGetOutput(ctx context.Context, command string) (val string, retErr error) {
	requiredMasks := []ECChannelName{ECChanSystem, ECChanUSB, ECChanMotionLid, ECChanMotionSense}
	var mask int64
	for _, name := range requiredMasks {
		if val, err := s.FindECChanMask(ctx, name); err != nil {
			return "", errors.Wrap(err, "FindECChanMask failed")
		} else {
			mask = mask | val
		}
	}
	if err := s.RunECCommand(ctx, "chan save"); err != nil {
		return "", errors.Wrap(err, "failed to send 'chan save' to EC")
	}
	if err := s.RunECCommand(ctx, fmt.Sprintf("chan %#x", mask)); err != nil {
		return "", errors.Wrapf(err, "failed to send 'chan %#x' to EC", mask)
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
			return "", &TabletModeCmdUnsupportedErr{E: errors.Errorf("device does not support tablet mode: %q", match)}
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
			if err := s.SetString(ctx, GSCTestlab, Open); err != nil {
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
	ECChanKeyboard    ECChannelName = "keyboard"
	ECChanMotionLid   ECChannelName = "motionlid"
	ECChanMotionSense ECChannelName = "motionsense"
	ECChanSwitch      ECChannelName = "switch"
	ECChanSystem      ECChannelName = "system"
	ECChanUSB         ECChannelName = "usb"
)

// FindECChanMask accepts an ec channel name, and runs ec 'chan' command to look for
// the corresponding mask value.
func (s *Servo) FindECChanMask(ctx context.Context, chanName ECChannelName) (maskVal int64, retErr error) {
	if s.ecChanMasks == nil {
		s.ecChanMasks = make(map[ECChannelName]int64)
	}
	if val, ok := s.ecChanMasks[chanName]; ok {
		return val, nil
	}
	if err := s.RunECCommand(ctx, "chan save"); err != nil {
		return 0, errors.Wrap(err, "failed to send 'chan save' to EC")
	}
	if err := s.RunECCommand(ctx, "chan 0"); err != nil {
		return 0, errors.Wrap(err, "failed to send 'chan 0' to EC")
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
		return 0, err
	}
	if out == nil || len(out[0]) < 2 {
		return 0, errors.Errorf("failed to parse chan output correctly, got: %v", out)
	}
	decimalVal, err := strconv.ParseInt(out[0][1], 16, 64)
	if err != nil {
		return 0, errors.Errorf("failed to parse mask value: %s, for ec chan: %s", out[0][1], string(chanName))
	}
	s.ecChanMasks[chanName] = decimalVal
	return decimalVal, nil
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
func (s *Servo) PressECBtnVerifyOutput(ctx context.Context, button DetachableECButton, duration int, ecChanMasks map[ECChannelName]string) (retErr error) {
	requiredMasks := []ECChannelName{ECChanKeyboard, ECChanSwitch}
	var mask int64
	for _, name := range requiredMasks {
		if val, err := s.FindECChanMask(ctx, name); err != nil {
			return errors.Wrap(err, "FindECChanMask failed")
		} else {
			mask = mask | val
		}
	}
	if err := s.RunECCommand(ctx, "chan save"); err != nil {
		return errors.Wrap(err, "failed to send 'chan save' to EC")
	}
	if err := s.RunECCommand(ctx, fmt.Sprintf("chan %#x", mask)); err != nil {
		return errors.Wrapf(err, "failed to send 'chan %#x' to EC", mask)
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

var keyboardReadyRe *regexp.Regexp = regexp.MustCompile(`KB enable`)

// WaitFirmwareKeyboardNoCmd waits until the DUT is in firmware with keyboard enabled
// On entry the DUT can be in firmware or kernel.
// This function works by waiting for the "KB enable" message shown by the EC when it boots.
// It does not issue commands to the EC.
func (s *Servo) WaitFirmwareKeyboardNoCmd(ctx context.Context, timeout time.Duration) (retErr error) {
	closeUART, err := s.EnableUARTCapture(ctx, ECUARTCapture)
	if err != nil {
		return errors.Wrap(err, "failed to enable capture EC UART")
	}
	defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }()
	if found, err := s.PollForRegexp(ctx, ECUARTStream, keyboardReadyRe, timeout); err != nil {
		return errors.Wrap(err, "gsc output parsing failed")
	} else if !found {
		return errors.Errorf("did not capture %s", keyboardReadyRe)
	}

	return nil
}

// GetSkuID runs the ec command 'cbi' and checks for the sku-id.
func (s *Servo) GetSkuID(ctx context.Context) (int, error) {
	skuID, err := s.RunECCommandGetOutput(ctx, "cbi", []string{reSKUID})
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(skuID[0][1])
}
