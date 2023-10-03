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

		if pdState.PEStateName != "PD_STATE_SNK_READY" {
			return errors.New("Servo charger port (C0) is not PD_STATE_SNK_READY")
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

// TriggerServoPDSoftReset triggers a USB-PD Soft Reset from the Servo-side
func (s *Servo) TriggerServoPDSoftReset(ctx context.Context) error {
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
			"SOFT_RESET",
			"SNK_DISCOVERYzz",
			"SNK_REQUESTED",
			"SNK_TRANSITION",
			"SNK_READY",
		}
	} else if pdState.PowerRole == PowerRoleSRC {
		expectedResetSequence = []string{
			"SOFT_RESET",
			"SRC_DISCOVERYxx",
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
	if checkSequenceInConsoleLog(out[0][0], 1, expectedResetSequence) {
		return errors.New("expected reset state sequence not seen in Servo PD soft reset command console output")
	}

	return nil
}
