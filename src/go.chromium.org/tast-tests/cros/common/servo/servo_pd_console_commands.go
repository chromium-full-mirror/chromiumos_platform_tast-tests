// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	servoPDStatePollTimeout  time.Duration = 5 * time.Second
	servoPDStatePollInterval time.Duration = 500 * time.Millisecond
)

// ServoSendDataSwapRequest initiates a data swap request from the servo's PD port.
func (s *Servo) ServoSendDataSwapRequest(ctx context.Context) (pdControlMsgType, error) {
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

	return PDCtrlReserved, errors.Errorf("unknown PD control message value %q", replyValue)
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

// verifyStatesInConsoleLog is a helper function which extracts all of the PD
// state messages from servo console output and then verifies the states match
// in exact order tp the states in the parameter sequenceList
func verifyStatesInConsoleLog(ctx context.Context, log string, port int, sequenceList []string) bool {
	// Create a regexp object that extracts all PD state entries from the log
	re := regexp.MustCompile(
		fmt.Sprintf(`C%d\s+st[\d]+\s([\w]+)`, port),
	)
	matches := re.FindAllStringSubmatch(log, -1)
	states := make([]string, len(matches))
	for idx, row := range matches {
		states[idx] = row[1]
		testing.ContextLogf(ctx, "act = %s <--> exp = %s", row[1], sequenceList[idx])
		// As long as the states have matched all the expected states,
		// then treat this as a match even if additional state messages
		// exist beyond what was expected.
		if idx >= len(sequenceList) {
			break
		}
		if states[idx] != sequenceList[idx] {
			testing.ContextLogf(ctx, "state list mismatch: %s", states)
			return false
		}
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

	// Run the command. Port 1 is the Servo's DUT-facing port.
	err = s.RunServoCommand(ctx, "pd 1 soft")
	if err != nil {
		return errors.Wrap(err, "could not trigger soft reset on Servo")
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

	// Depending on the current power role, set the list of expected
	// PD states following the soft reset
	var expectedResetSequence []string
	if pdState.PowerRole == PowerRoleSNK {
		expectedResetSequence = []string{
			"HARD_RESET_SEND",
			"HARD_RESET_EXECUTE",
			"SNK_HARD_RESET_RECOVER",
			"SNK_DISCOVERY",
			"SNK_REQUESTED",
			"SNK_TRANSITION",
			"SNK_READY",
		}
	} else if pdState.PowerRole == PowerRoleSRC {
		expectedResetSequence = []string{
			"HARD_RESET_SEND",
			"HARD_RESET_EXECUTE",
			"SRC_HARD_RESET_RECOVER",
			"SRC_STARTUP",
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
	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 hard", []string{`(.*)(C1)\s+[\w]+:?\s([\w]+_READY)`})
	if err != nil {
		return errors.Wrap(err, "could not trigger hard reset on Servo")
	}
	// Verify hard reset happened and that the connection recovers as expected
	if !verifyStatesInConsoleLog(ctx, out[0][0], 1, expectedResetSequence) {
		return errors.New("expected reset state sequence not seen in Servo console output")
	}

	// Hard reset should result in the same power after as before, but the data
	// role may be different as it be the data role associated with the power
	// role. Poll here to wait for the data role after the hard reset to be
	// the same as before to ensure the PD connection is back to its steady
	// state condition
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
