// Copyright 2023 The ChromiumOS Authors
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

// HPDLevelValue is a type for storing a type-c alt state hpd level
type HPDLevelValue string

// Supported hpd levels
const (
	HPDHigh HPDLevelValue = "h"
	HPDLow  HPDLevelValue = "l"
	HPDExt  HPDLevelValue = "ext"
	HPDirq  HPDLevelValue = "irq"
)

// USBCCurrentAdvertisement is a type for storing values of current advertisement without pd comm
type USBCCurrentAdvertisement string

// Supported usbc current advertisement
const (
	USBC3A0 USBCCurrentAdvertisement = "3A0"
	USBC1A5 USBCCurrentAdvertisement = "1A5"
	USBCusb USBCCurrentAdvertisement = "USB"
)

// INAInfo contains all the information retrieved from the INA
type INAInfo struct {
	ShuntUV   int // shunt voltage in uV
	BusMV     int // bus voltage in mV
	PowerMW   int // power in mW
	CurrentMA int // current in mA
}

const (
	servoPDStatePollTimeout  time.Duration = 5 * time.Second
	servoPDStatePollInterval time.Duration = 500 * time.Millisecond
)

// ServoSendDataSwapRequest initiates a data swap request from the servo's PD port.
func (s *Servo) ServoSendDataSwapRequest(ctx context.Context) (pdControlMsgType, error) {
	if err := s.EnableServoConsoleChannel(ctx, "usbpd"); err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to enable usbpd logging channel")
	}
	// Enable PD message so we can check the response from the DUT.
	err := s.RunServoCommand(ctx, "pd dump 2")
	if err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to send enable PD debug")
	}
	// Always disable PD commands on exit.
	defer s.RunServoCommand(ctx, "pd dump 0")

	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 swap data", []string{reEcPdRecv})
	if err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to send servo data swap")
	}

	recvMsg, err := strconv.ParseUint(out[0][1], 16, 32)
	if err != nil {
		return PDCtrlReserved, errors.Wrapf(err, "failed to convert swap RECV message %q", out[0][1])
	}

	replyValue := int(recvMsg & PdControlMsgMask)
	if reply, ok := pdControlMsg[replyValue]; ok {
		return reply, nil
	}

	return PDCtrlReserved, errors.Errorf("unknown PD control message value %v", replyValue)
}

// ServoSetDataSwapReject sets servo policy of acceptance of Data Role Swap Requests
func (s *Servo) ServoSetDataSwapReject(ctx context.Context, RejectSwap bool) error {
	action := 1
	if RejectSwap {
		action = 0
	}

	if err := s.RunServoCommand(ctx, "usbc_action drswap "+strconv.Itoa(action)); err != nil {
		return errors.Wrap(err, "failed to set action for servo drswap")
	}

	return nil
}

// ServoSendPowerSwapRequest sends power swap request to be initiated by the Servo.
func (s *Servo) ServoSendPowerSwapRequest(ctx context.Context) (pdControlMsgType, error) {
	// Enable PD message so we can check the response from the DUT.
	err := s.RunServoCommand(ctx, "pd dump 2")
	if err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to send enable PD debug")
	}
	// Always disable PD commands on exit.
	defer s.RunServoCommand(ctx, "pd dump 0")

	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 swap power", []string{reEcPdRecv})
	if err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to send servo data swap")
	}

	recvMsg, err := strconv.ParseUint(out[0][1], 16, 32)
	if err != nil {
		return PDCtrlReserved, errors.Wrapf(err, "failed to convert swap RECV message %q", out[0][1])
	}

	replyValue := int(recvMsg & PdControlMsgMask)
	if reply, ok := pdControlMsg[replyValue]; ok {
		return reply, nil
	}

	return PDCtrlReserved, errors.Errorf("unknown PD control message value %v", replyValue)
}

// ServoGetDualRoleState accepts a port ID and checks for the PD DRP status of this port.
func (s *Servo) ServoGetDualRoleState(ctx context.Context) (USBPdDualRoleValue, error) {
	port := 1

	matchList := []string{`dual-role toggling:\s+([\w ]+)[\r\n]`}

	// Try modern `pd N dualrole` command
	cmd := fmt.Sprintf("pd %d dualrole", port)
	out, err := s.RunServoCommandGetOutput(ctx, cmd, matchList)
	if err != nil {
		testing.ContextLogf(ctx, "EC command %q failed. Trying older version. (%q)",
			cmd, err)

		// Older DUTs running firmware from before cl:1096654 don't have per-port
		// dualrole settings. Fall back to the old command.
		out, err = s.RunServoCommandGetOutput(ctx, "pd dualrole", matchList)
		if err != nil {
			// Servo does not support DRP
			return "", errors.Wrapf(err, "ec command %q failed. No way to check dual role state", cmd)
		}
	}
	testing.ContextLogf(ctx, "Port %d DRP status: %q", port, out[0][1])
	return USBPdDualRoleValue(out[0][1]), nil
}

func (s *Servo) toggleServoDualRole(ctx context.Context) (int, error) {
	drpCmd := "usbc_action drp"
	drpRe := []string{`DRP\s=\s(\d)`}

	// Send DRP toggle command to PDTester and get value of 'drp_enable'
	testing.ContextLogf(ctx, "PD Tester running: %s", drpCmd)
	out, err := s.RunServoCommandGetOutput(ctx, drpCmd, drpRe)
	if err != nil {
		return 0, errors.Wrapf(err, "command %q failed", drpCmd)
	}
	i, err := strconv.Atoi(out[0][1])
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert string to int")
	}
	return i, nil
}

func (s *Servo) enableServoDualRole(ctx context.Context) error {
	for i := 0; i < 2; i++ {
		if r, _ := s.toggleServoDualRole(ctx); r == 1 {
			testing.ContextLog(ctx, "PDTester DRP mode enabled")
			return nil
		}
	}
	testing.ContextLog(ctx, "PDTester DRP mode set failure")
	return nil
}

// ServoSetDualRole sets the PD DRP status of this port
func (s *Servo) ServoSetDualRole(ctx context.Context, val USBPdDualRoleValue) error {
	// USBPdDualRoleSink and Source contain "force " prefix, strip this from the command
	// sent to servo
	action := strings.TrimPrefix(string(val), "force ")

	state, _ := s.ServoGetDualRoleState(ctx)

	if state == val {
		return nil
	}

	if val == USBPdDualRoleOn {
		s.enableServoDualRole(ctx)
	} else {
		if state == USBPdDualRoleOn {
			_, err := s.toggleServoDualRole(ctx)
			return err
		}

		cmd := fmt.Sprintf("pd 1 dualrole %s", action)
		testing.ContextLogf(ctx, "PD Tester running: %s", cmd)
		if err := s.RunServoCommand(ctx, cmd); err != nil {
			testing.ContextLogf(ctx, "EC command %q failed. Trying older version. (%q)",
				cmd, err)

			cmd := fmt.Sprintf("pd dualrole %s", action)

			if err := s.RunServoCommand(ctx, cmd); err != nil {
				return errors.Wrapf(err, "ec command %q failed", cmd)
			}
		}
	}

	state, _ = s.ServoGetDualRoleState(ctx)

	if state != val {
		return errors.Errorf("failed to set dual role to %q", val)
	}

	return nil
}

// ServoSetDPConfigs sets the DP configs for the DUT connection
func (s *Servo) ServoSetDPConfigs(ctx context.Context, config *TypeCInfo, mfPref MultiFunctionPref) error {
	// we'll lose the existing connection status while trying to set DP mode.
	// Get current state to restore it later.
	savedpdState, err := s.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to store current pd state before turning on DP alt")
	}

	if err := s.ServoCcOff(ctx); err != nil {
		return errors.Wrap(err, "failed to turn off cc")
	}

	if err := s.RunServoCommand(ctx, "usbc_action dp disable"); err != nil {
		return errors.Wrap(err, "failed to disable DP alt-mode")
	}

	if config.DPMode == DPEnable {
		testing.ContextLog(ctx, "Resetting usb-c connection to DP alt-mode")

		if err := s.RunServoCommand(ctx, fmt.Sprintf("usbc_action dp pins %s", config.PinsCDEF)); err != nil {
			return errors.Wrap(err, "failed to set pin assignments")
		}

		if err := s.RunServoCommand(ctx, fmt.Sprintf("usbc_action dp mf %d", mfPref)); err != nil {
			return errors.Wrap(err, "failed to set mf pref")
		}

		if err := s.RunServoCommand(ctx, "usbc_action dp enable"); err != nil {
			return errors.Wrap(err, "failed to enable DP alt-mode")
		}
	}

	if err := s.SetPDRole(ctx, PDRoleSrc); err != nil {
		return errors.Wrap(err, "failed to set pd role")
	}

	if err := s.SetPDCommunication(ctx, On); err != nil {
		return errors.Wrap(err, "failed to enable pd comms")
	}

	// Poll on the Servo C1 (DUT-facing) port until it is source-ready
	prevIsSourceReady := false
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := s.GetServoPDState(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		testing.ContextLogf(ctx, "Servo DUT port PE State: %s", pdState.PEStateName)

		isSourceReady := pdState.IsSourceReady()
		if !(isSourceReady && prevIsSourceReady) {
			// Require two consecutive reads to be src-ready to make sure the
			// connection has stabilized.
			prevIsSourceReady = isSourceReady

			return errors.New("Servo DUT port (C1) is not src-ready")
		}

		return nil
	}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "timed out waiting for Servo DUT port to be ready")
	}

	// If required role is sink, have servo initiate a power swap
	if savedpdState.IsSinkReady() {
		if _, err := s.ServoSendPowerSwapRequest(ctx); err != nil {
			return errors.Wrap(err, "swap power failed")
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			pdState, err := s.GetServoPDState(ctx)
			if err != nil {
				return testing.PollBreak(
					errors.Wrap(err, "cannot access servo DUT port (C1) PD status"),
				)
			}

			testing.ContextLogf(ctx, "Servo DUT port (C1) PE State is %s", pdState.PEStateName)

			if !pdState.IsSinkReady() {
				return errors.New("Servo DUT port (C1) is not sink-ready")
			}

			return nil
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			return errors.Wrap(err, "timed out waiting for servo DUT port to sink power")
		}
		// servo does not broadcast that it's USB capable in sink mode while dp alt mode is disabled
		// but pd comm is enabled, which in some cases can cause the DUT to lose ethernet.
		if config.DPMode == DPDisable {
			if s.RunServoCommand(ctx, "cc snk"); err != nil {
				return errors.Wrap(err, "failed to disable pd comms")
			}
		}
	}

	return nil
}

// SetHPD sets the HPD value for an active dp-alt connection
func (s *Servo) SetHPD(ctx context.Context, HPDLevel HPDLevelValue) error {
	if err := s.RunServoCommand(ctx, fmt.Sprintf("usbc_action dp hpd %s", HPDLevel)); err != nil {
		return errors.Wrap(err, "failed to set hpd level")
	}

	return nil
}

// SetPlug sets the PLug status for an active dp-alt connection
func (s *Servo) SetPlug(ctx context.Context, Plug bool) error {
	cmd := "usbc_action dp plug 0"
	if Plug {
		cmd = "usbc_action dp plug 1"
	}
	if err := s.RunServoCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "failed to set plug status")
	}

	// we only use commands off and on to preserve the usbc state between resets
	if err := s.ServoCcOff(ctx); err != nil {
		return errors.Wrap(err, "failed to turn off cc")
	}

	if err := s.ServoCcOn(ctx); err != nil {
		return errors.Wrap(err, "failed to turn on cc")
	}

	return nil
}

// ServoSetUSBVersion3 sets the DUT USB connection as version 2.0 or 3.0
func (s *Servo) ServoSetUSBVersion3(ctx context.Context, USBVersion3 bool) error {
	enableVersion3 := "disable"

	if USBVersion3 {
		enableVersion3 = "enable"
	}

	cmd := fmt.Sprintf("dut_usb3 %s", enableVersion3)
	if err := s.RunServoCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "unable to set usb version")
	}

	return nil
}

// RequireChargerAttached verifies that the Servo charger port (#0) is an active sink
func (s *Servo) RequireChargerAttached(ctx context.Context) error {

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := s.GetServoChargerPortPDState(ctx)
		if err != nil {
			return testing.PollBreak(
				errors.Wrap(err, "cannot access charger port PD status"),
			)
		}

		testing.ContextLogf(ctx, "C0 PE State is %s", pdState.PEStateName)

		if !pdState.IsSinkReady() {
			return errors.New("Servo charger port (C0) is not sink-ready")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return err
	}

	return nil
}

// EnableServoConsoleChannel enables the provided console log channel (and turns all but the console CLI off)
func (s *Servo) EnableServoConsoleChannel(ctx context.Context, channelName string) error {
	// Get current console state
	//  0 - full row
	//  1 - channel index
	//  2 - channel mask
	//  3 - enabled flag
	//  4 - channel name
	out, err := s.RunServoCommandGetOutput(ctx, "chan", []string{fmt.Sprintf(`(\d+)\s+(\w+)\s+(\*?)\s+%s[\r\n]`, channelName)})
	if err != nil {
		return errors.Wrapf(err, "failed to query channel %q", channelName)
	}

	mask, err := strconv.ParseUint(out[0][2], 16, 32)
	if err != nil {
		return errors.Wrap(err, "cannot parse channel mask")
	}

	if out[0][3] == "*" {
		// No update necessary
		testing.ContextLogf(ctx, "Servo console channel %q already enabled", channelName)
		return nil
	}

	testing.ContextLogf(ctx, "Enabling Servo console channel %q", channelName)
	return s.RunServoCommand(ctx, fmt.Sprintf("chan 0x%08x", mask))
}

// EnableServoPDConsoleDebug enables PD console debugging level 2 on the Servo
func (s *Servo) EnableServoPDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 2"

	testing.ContextLog(ctx, "Enabling Servo PD Console Debug")
	if err := s.RunServoCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "servo pd command failed")
	}

	return nil
}

// DisableServoPDConsoleDebug disables PD console debugging on the Servo
func (s *Servo) DisableServoPDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 0"

	testing.ContextLog(ctx, "Disabling Servo PD Console Debug")
	if err := s.RunServoCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "servo pd command failed")
	}

	return nil
}

// checkSequenceInConsoleLog is a helper function to search through console
// output and ensure a provided sequence of PD state transitions exists.
func checkSequenceInConsoleLog(log string, port int, sequenceList []string) bool {
	i := 0
	for _, stateName := range sequenceList {
		// Create a regexp object that matches the expected log entry
		// for this state name.
		re := regexp.MustCompile(
			fmt.Sprintf(`C%d\s+[\w]+:?\s(%s)`, port, stateName),
		)
		idx := re.FindStringIndex(log[i:])

		if idx == nil {
			return false
		}

		// Continue the search for the next expected state after this
		// log line
		i += idx[1]
	}
	return true
}

// verifyStatesInConsoleLog is a helper function which extracts all of the PD
// state messages from servo console output and then verifies the states match
// in exact order tp the states in the parameter sequenceList
func verifyStatesInConsoleLog(ctx context.Context, log string, port int, sequenceList []string, expectedFinalState string) bool {
	// Create a regexp object that extracts all PD state entries from the log
	re := regexp.MustCompile(
		fmt.Sprintf(`C%d\s+st[\d]+\s([\w]+)`, port),
	)
	matches := re.FindAllStringSubmatch(log, -1)
	states := make([]string, len(matches))
	for idx, row := range matches {
		states[idx] = row[1]
		// Continue capturing all state names so we can check the expected final state below
		if idx >= len(sequenceList) {
			continue
		}
		testing.ContextLogf(ctx, "act = %s <--> exp = %s", row[1], sequenceList[idx])

		if states[idx] != sequenceList[idx] {
			testing.ContextLogf(ctx, "state list mismatch: %s", states)
			return false
		}
	}

	testing.ContextLogf(ctx, "Final state: act = %s <--> exp = %s", states[len(states)-1], expectedFinalState)

	if states[len(states)-1] != expectedFinalState {
		testing.ContextLogf(ctx, "final state mismatch: %s", states)
		return false
	}

	return true
}

// TriggerServoPDSoftReset triggers a USB-PD Soft Reset from the Servo-side
func (s *Servo) TriggerServoPDSoftReset(ctx context.Context) error {
	// Get current port status
	pdStateBefore, err := s.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get Servo PD state")
	}

	if pdStateBefore.Connection != PDEnabled {
		return errors.New("servo PD status reads disabled. Cannot test without a port partner")
	}

	if err := s.EnableServoConsoleChannel(ctx, "usbpd"); err != nil {
		return err
	}

	if err := s.EnableServoPDConsoleDebug(ctx); err != nil {
		return errors.Wrap(err, "could not enable Servo's PD debug logs")
	}

	// Go back to `pd dump 0` after.
	defer s.DisableServoPDConsoleDebug(ctx)

	// Depending on the current power role, set the list of expected
	// PD states following the soft reset
	var expectedResetSequence []string
	if pdStateBefore.PowerRole == PowerRoleSNK {
		expectedResetSequence = []string{
			"SOFT_RESET",
			"SNK_DISCOVERY",
			"SNK_REQUESTED",
			"SNK_TRANSITION",
			"SNK_READY",
		}
	} else if pdStateBefore.PowerRole == PowerRoleSRC {
		expectedResetSequence = []string{
			"SOFT_RESET",
			"SRC_DISCOVERY",
			"SRC_NEGOCIATE", // [sic]
			"SRC_ACCEPTED",
			"SRC_POWERED",
			"SRC_TRANSITION",
			"SRC_READY",
		}
	} else {
		return errors.New("unknown power role state")
	}

	// Run the command
	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 soft", []string{`(.*)(C1)\s+[\w]+:?\s([\w]+_READY)`})
	if err != nil {
		return errors.Wrap(err, "could not trigger soft reset on Servo")
	}
	if !checkSequenceInConsoleLog(out[0][0], 1, expectedResetSequence) {
		return errors.New("expected reset state sequence not seen in Servo PD soft reset command console output")
	}

	// Poll until the pre- and post-reset states match or we time out.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdStateAfter, err := s.GetServoPDState(ctx)
		if err != nil {
			return errors.Wrap(err, "cannot read servo PD state")
		}

		return pdStateBefore.Compare(pdStateAfter)
	}, &testing.PollOptions{Timeout: pdStatePollTimeout, Interval: pdStatePollInterval}); err != nil {
		return errors.Wrap(err, "timed out waiting for states to match after servo soft reset")
	}

	// TODO (b/317808083) query the servo's soft reset counter here

	return nil
}

// TriggerServoPDHardReset triggers a USB-PD Hard Reset from the Servo-side
func (s *Servo) TriggerServoPDHardReset(ctx context.Context) error {
	// Get current port status
	pdState, err := s.GetServoPDState(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get Servo PD state")
	}

	if pdState.Connection != PDEnabled {
		return errors.New("servo PD status reads disabled. Cannot test without a port partner")
	}

	if err := s.EnableServoPDConsoleDebug(ctx); err != nil {
		return errors.Wrap(err, "could not enable Servo's PD debug logs")
	}

	// Go back to `pd dump 0` after.
	defer s.DisableServoPDConsoleDebug(ctx)

	// GoBigSleepLint: Let PD state settle before triggering hard reset.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	// Depending on the current power role, set the list of expected
	// PD states following the soft reset
	var expectedResetSequence []string
	var expectedFinalState string
	if pdState.PowerRole == PowerRoleSNK {
		expectedResetSequence = []string{
			"HARD_RESET_SEND",
			"HARD_RESET_EXECUTE",
			"SNK_HARD_RESET_RECOVER",
		}
		expectedFinalState = "SNK_READY"
	} else if pdState.PowerRole == PowerRoleSRC {
		expectedResetSequence = []string{
			"HARD_RESET_SEND",
			"HARD_RESET_EXECUTE",
			"SRC_HARD_RESET_RECOVER",
		}
		expectedFinalState = "SRC_READY"
	} else {
		return errors.New("unknown power role state")
	}

	// Capture servo output for 5 seconds. Verify that after sending the
	// hard reset, servo progresses through the hard reset handling and
	// that the final servo state, SNK_READY or SRC_READY is expected.
	output, err := s.CaptureServoCommand(ctx, "pd 1 hard", 5*time.Second)
	if err != nil {
		return errors.New("failed to send hard reset command to servo")
	}

	testing.ContextLog(ctx, "Hard reset output :", output)

	// Verify hard reset happened and that the connection recovers as expected
	if !verifyStatesInConsoleLog(ctx, output, 1, expectedResetSequence, expectedFinalState) {
		return errors.New("expected reset state sequence not seen in Servo console output")
	}

	// Hard reset should result in the same power after as before, but the data
	// role may be different as it may be the data role associated with the power
	// role. Poll here to wait for the data role after the hard reset to be
	// the same as before to ensure the PD connection is back to its steady
	// state condition.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if pdStateAfter, err := s.GetServoPDState(ctx); err == nil {
			if pdState.DataRole != pdStateAfter.DataRole {
				return errors.Wrap(err, "Data role does not match expected")
			}
		} else {
			return errors.Wrap(err, "failed to get Servo PD state")
		}

		return nil
	}, &testing.PollOptions{Timeout: servoPDStatePollTimeout, Interval: servoPDStatePollInterval}); err != nil {
		return errors.Wrap(err, "Data roles did not match following servo initiated hard reset")
	}

	return nil
}

// ServoCcOff runs the `cc off` console command on the Servo.
func (s *Servo) ServoCcOff(ctx context.Context) error {
	output, err := s.RunServoCommandGetOutput(ctx, "cc off", []string{`cc: (\w+)[\r\n]`})

	if err == nil && output[0][1] != "off" {
		return errors.New("CC state did not change to 'off'")
	}

	return err
}

// ServoCcOn runs the `cc on` console command on the Servo.
func (s *Servo) ServoCcOn(ctx context.Context) error {
	output, err := s.RunServoCommandGetOutput(ctx, "cc on", []string{`cc: (\w+)[\r\n]`})

	if err == nil && output[0][1] != "on" {
		return errors.New("CC state did not change to 'on'")
	}

	return err
}

// ServoCcSnk runs the `cc snk` console command on the Servo.
func (s *Servo) ServoCcSnk(ctx context.Context) error {
	output, err := s.RunServoCommandGetOutput(ctx, "cc snk", []string{`cc: (\w+)[\r\n]`})

	if err == nil && output[0][1] != "on" {
		return errors.New("CC state did not change to 'on': " + output[0][1])
	}

	return err
}

// ServoCcSrc runs the `cc src` console command on the Servo, optionally with dts enabled.
// Source role with dts enabled is default servo cc state.
func (s *Servo) ServoCcSrc(ctx context.Context, dts bool) error {
	dtsString := ""
	if dts {
		dtsString = "dts"
	}
	cmd := fmt.Sprintf("cc src%s", dtsString)
	err := s.RunServoCommand(ctx, cmd)

	return err
}

// ServoCcDac runs the `cc_dac` console command on the Servo.
func (s *Servo) ServoCcDac(ctx context.Context, cc int, param string) error {
	cmd := fmt.Sprintf("cc_dac %d %s", cc, param)
	err := s.RunServoCommand(ctx, cmd)

	if err != nil {
		return errors.Wrap(err, "failed to run cc_dac cmd")
	}

	return err
}

// ServoCCNoPD connects the servo to the DUT as a source without PD communication
func (s *Servo) ServoCCNoPD(ctx context.Context, dts bool, connectionType USBCCurrentAdvertisement) error {
	dtsString := ""
	if dts {
		dtsString = "dts"
	}
	cmd := fmt.Sprintf("cc nopdsrc%s%s", dtsString, string(connectionType))

	if err := s.RunServoCommand(ctx, cmd); err != nil {
		testing.ContextLog(ctx, "Could not run servo command, are you on the right version?")
		testing.ContextLog(ctx, "Try updating servo \"servo_updater --updater_channel latest -- -b servo_v4p1 -c alpha\"")
		return errors.Wrap(err, "could not run command to enable connection without pd, try verifying servo FW version")
	}

	return nil
}

var reEcSBU string = `SBU1_DET = ([\d]+) mV[\s].*SBU2_DET = ([\d]+) mV`

// ServoGetSBU retrieves the SBU voltages
func (s *Servo) ServoGetSBU(ctx context.Context) ([]int, error) {
	ret := []int{0, 0}

	out, err := s.RunServoCommandGetOutput(ctx, "adc", []string{reEcSBU})
	if err != nil {
		return ret, errors.Wrap(err, "failed to send adc command")
	}

	ret[0], err = strconv.Atoi(out[0][1])
	if err != nil {
		return ret, errors.Wrap(err, "failed to retrieve SBU voltage")
	}

	ret[1], err = strconv.Atoi(out[0][2])
	if err != nil {
		return ret, errors.Wrap(err, "failed to retrieve SBU voltage")
	}

	return ret, nil
}

var reServoINA string = `Shunt voltage.*?([\d]*) uV.*[\s]*.*Bus voltage.*?([\d]*) mV.*[\s]*.*Power.*?([\d]*) mW.*[\s]*.*Current.*?([\d]*) mA`

// ServoGetINA retrieves the voltage and current of a port from the INA
func (s *Servo) ServoGetINA(ctx context.Context, port int) (INAInfo, error) {
	ret := INAInfo{0, 0, 0, 0}
	cmd := fmt.Sprintf("ina %d", port)

	out, err := s.RunServoCommandGetOutput(ctx, cmd, []string{reServoINA})
	if err != nil {
		return ret, errors.Wrap(err, "failed to send ina command")
	}

	ret.ShuntUV, err = strconv.Atoi(out[0][1])
	if err != nil {
		return ret, errors.Wrap(err, "failed to retrieve shunt voltage")
	}

	ret.BusMV, err = strconv.Atoi(out[0][2])
	if err != nil {
		return ret, errors.Wrap(err, "failed to retrieve bus voltage")
	}

	ret.PowerMW, err = strconv.Atoi(out[0][3])
	if err != nil {
		return ret, errors.Wrap(err, "failed to retrieve power")
	}

	ret.CurrentMA, err = strconv.Atoi(out[0][4])
	if err != nil {
		return ret, errors.Wrap(err, "failed to retrieve current")
	}

	return ret, nil
}

// ServoGetConnectedStateAfterCCReconnect get the connected state after disconnect/reconnect using PDTester
//
// PDTester supports a feature which simulates a USB Type C disconnect
// and reconnect. It returns the first connected state (either source or
// sink) after reconnect.
//
// @param disconnectTime: Time in seconds for disconnect period.
// @returns: The connected PD state.
func (s *Servo) ServoGetConnectedStateAfterCCReconnect(ctx context.Context, disconnectTime time.Duration) (string, error) {
	discDelay := 100
	port := 1
	cmd := fmt.Sprintf("fakedisconnect %d %d", discDelay, disconnectTime.Milliseconds())

	srcConnect := []string{"SRC_READY"}
	snkConnect := []string{"SNK_READY"}
	srcDisc := "SRC_DISCONNECTED"
	sinkDisc := "SNK_DISCONNECTED"
	drpAutoToggle := "DRP_AUTO_TOGGLE"

	stateExp := `(C%d)\s+[\w]+:?\s(%s)`

	disconnectedStates := strings.Join([]string{srcDisc, sinkDisc, drpAutoToggle}, `|`)
	disconnectedExp := fmt.Sprintf(stateExp, port, disconnectedStates)

	connectedStates := strings.Join(append(srcConnect, snkConnect...), `|`)
	connectedExp := fmt.Sprintf(stateExp, port, connectedStates)

	if err := s.EnableServoPDConsoleDebug(ctx); err != nil {
		return "", errors.Wrap(err, "could not enable Servo's PD debug logs")
	}

	// Go back to `pd dump 0` after.
	defer s.DisableServoPDConsoleDebug(ctx)

	output, err := s.RunServoCommandGetOutput(ctx, cmd, []string{disconnectedExp, connectedExp})

	if err != nil {
		return "", errors.Wrap(err, "failed to run fakedisconnect cmd")
	}

	return output[1][2], nil
}
