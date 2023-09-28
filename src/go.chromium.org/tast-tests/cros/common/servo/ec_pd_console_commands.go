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
	// reEcPdStateCommand is a TCPM v1 and v2 compatible regex for pd <port> state output.
	//   Example: Port C0 CC3, Enable - Role: SRC-DFP TC State: Attached.SRC, Flags: 0x9002 PE State: PE_SRC_Ready, Flags: 0x0201
	//   Match Index:
	//      0 - Full match
	//      1 - Port number  -- 0
	//      2 - Polarity     -- 3
	//      3 - Comm Status  -- Enable
	//      4 - Power role   -- SRC
	//      5 - Data role    -- DFP
	//      6 - TCPM version-specific extra fields
	reEcPdStateCommand string = `Port\s+C(\d+)\s+CC(\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)(.*)[\r\n]`
	reEcPdRecv         string = `RECV\s([\w]+)`
	rePDVersion        string = `\s+(\d+|Wrong.*)`
	// MaxPorts specifies the maximum number of ports on the EC.
	MaxPorts int = 4
	// PDPortUnderTest indicates command should be sent to the PD port connected to servo.
	PDPortUnderTest int = MaxPorts
)

// DUTPDInfo caches the fixed PD testing information for the DUT.
type DUTPDInfo struct {
	version    int // 1==TCPMv1, 2==TCPMv1
	activePort int // PD port connected to servo
	portCount  int // Total number of PD ports on the DUT
}

// extractPEStateNameTCPMv1 interprets console output on TCPMv1 DUTs and extracts
// the current PE State and converts it to a string name
func extractPEStateNameTCPMv1(extraFields string) (string, error) {
	re := regexp.MustCompile(`State: (\d+)`)

	matches := re.FindStringSubmatch(extraFields)
	if len(matches) < 1 {
		return "", errors.Errorf("cannot extract state from %q", extraFields)
	}

	stateNum, err := strconv.Atoi(matches[1])
	if err != nil {
		return "", errors.Wrap(err, "cannot convert PE state")
	}

	stateName, ok := peStateNameLookup[stateNum]
	if !ok {
		return "", errors.Errorf("unknown PE state %d", stateNum)
	}
	return stateName, nil
}

// extractPEStateNameTCPMv2 interprets console output on TCPMv2 DUTs and extracts
// the current PE State name
func extractPEStateNameTCPMv2(extraFields string) (string, error) {
	// The match group is optional because it is not present if PD comms are disabled
	re := regexp.MustCompile(`PE State: ([A-Za-z_]+)?,`)

	matches := re.FindStringSubmatch(extraFields)
	if len(matches) < 1 {
		return "", errors.Errorf("cannot extract state from %q", extraFields)
	}

	return matches[1], nil
}

// RequireDUTPDInfo allocates and caches the fixed information about the PD port under test.
func (s *Servo) RequireDUTPDInfo(ctx context.Context) error {
	if s.dutPDInfo != nil {
		return nil
	}

	pdInfo := &DUTPDInfo{}

	out, err := s.RunECCommandGetOutput(ctx, "pd version", []string{rePDVersion})
	if err != nil {
		return errors.Wrap(err, "EC pd version failed")
	}

	pdInfo.version, err = strconv.Atoi(out[0][1])
	if err != nil {
		testing.ContextLog(
			ctx,
			"PD Version command is not supported. This test is likely running "+
				"against an old version of the EC. The test will assume the "+
				" DUT is running the TCPMv1 stack. This may cause errors.",
		)
		pdInfo.version = 1
	}

	if !(pdInfo.version == 1 || pdInfo.version == 2) {
		panic("Unsupported TCPM version")
	}

	numPorts := 0
	enabledPorts := 0
	pdPort := MaxPorts
	for port := 0; port < MaxPorts; port++ {
		if out, err := s.GetPDState(ctx, port); err == nil {
			testing.ContextLogf(ctx, "DUT Port %d state: %q", port, out)

			// Element #6 has the TCPM version-specific output:
			//  - For TCPMv1: "State: 8(), Flags: 0x16946"
			//  - For TCPMv2: "TC State: Attached.SRC, Flags: 0x9002 PE State: PE_SRC_Ready, Flags: 0x0201"
			extraFields := out[0][6]

			var stateName string
			var err error

			if pdInfo.version == 1 {
				stateName, err = extractPEStateNameTCPMv1(extraFields)
			} else if pdInfo.version == 2 {
				stateName, err = extractPEStateNameTCPMv2(extraFields)
			}

			if err != nil {
				return errors.Wrapf(err, "cannot read TCPMv%d state", pdInfo.version)
			}

			var activePEStates = map[string]bool{
				"PD_STATE_SNK_READY": true,
				"PD_STATE_SRC_READY": true,
				"PE_SNK_Ready":       true,
				"PE_SRC_Ready":       true,
			}

			if _, ok := activePEStates[stateName]; ok {
				pdPort = port
				enabledPorts++
			}
			numPorts++
		} else {
			testing.ContextLogf(ctx, "DUT Port %d not present", port)
		}
	}

	if numPorts == 0 {
		return errors.New("no PD ports found on the DUT")
	}
	pdInfo.portCount = numPorts

	if enabledPorts == 0 {
		return errors.New("no active PD ports found")
	} else if enabledPorts > 1 {
		return errors.New("more than one active PD port found")
	}
	pdInfo.activePort = pdPort

	testing.ContextLogf(ctx, "DUT PD Port info: TCPMv%d, testing port %d, port count %d",
		pdInfo.version, pdInfo.activePort, pdInfo.portCount)

	s.dutPDInfo = pdInfo

	return nil
}

// GetPDState returns PD state console output for a PD port on the DUT.
func (s *Servo) GetPDState(ctx context.Context, port int) ([][]string, error) {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	} else if port > MaxPorts {
		return nil, errors.Errorf("invalid PD port number %d", port)
	}
	cmd := fmt.Sprintf("pd %d state", port)

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{reEcPdStateCommand})
	if err != nil {
		return nil, errors.Wrap(err, "EC pd command failed")
	}

	return out, nil
}

const (
	pdStatePollTimeout  time.Duration = 10 * time.Second
	pdStatePollInterval time.Duration = 500 * time.Millisecond
)

// SendPowerSwapRequest sends power swap request to be initiated by the DUT.
func (s *Servo) SendPowerSwapRequest(ctx context.Context, port int) error {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	} else if port > MaxPorts {
		return errors.Errorf("invalid PD port number %d", port)
	}
	cmd := fmt.Sprintf("pd %d swap power", port)

	s.EnablePDConsoleDebug(ctx)
	defer s.DisablePDConsoleDebug(ctx)

	testing.ContextLog(ctx, "Sending power swap request: ", cmd)

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{reEcPdRecv})

	if err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}
	testing.ContextLog(ctx, "PowerSwap reply: ", out)

	return nil
}

// SendDataSwapRequest sends data swap request to be initiated by the DUT.
func (s *Servo) SendDataSwapRequest(ctx context.Context, port int) error {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	} else if port > MaxPorts {
		return errors.Errorf("invalid PD port number %d", port)
	}
	cmd := fmt.Sprintf("pd %d swap data", port)

	s.EnablePDConsoleDebug(ctx)
	defer s.DisablePDConsoleDebug(ctx)

	testing.ContextLog(ctx, "Sending data swap request: ", cmd)

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{reEcPdRecv})

	if err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}
	testing.ContextLog(ctx, "DataSwap reply: ", out)

	return nil
}

// EnablePDConsoleDebug enables PD console debugging level 2 on the DUT.
func (s *Servo) EnablePDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 2"

	testing.ContextLog(ctx, "Enabling PD Console Debug")
	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// DisablePDConsoleDebug disables PD console debugging on the DUT.
func (s *Servo) DisablePDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 0"

	testing.ContextLog(ctx, "Disabling PD Console Debug")
	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// GetDualRole returns true if dual role power is enabled on port.
func (s *Servo) GetDualRole(ctx context.Context, port int) (bool, error) {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	}
	cmd := fmt.Sprintf("pd %d dualrole", port)
	onResponse := "on"

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{`dual-role toggling:\s+([\w ]+)[\r\n]`})

	if err != nil {
		return false, errors.Wrap(err, "EC pd command failed")
	}
	testing.ContextLog(ctx, "DualRole reply: ", out[0][1])

	return out[0][1] == onResponse, nil
}

// SetPDPowerRole sets the PD power role for a PD port on the DUT.
func (s *Servo) SetPDPowerRole(ctx context.Context, port int, role string) error {
	out, err := s.GetPDState(ctx, port)

	if err != nil {
		return errors.Wrap(err, "failed to get PD State")
	}

	if out[0][4] != role {
		if err := s.SendPowerSwapRequest(ctx, port); err != nil {
			return errors.Wrap(err, "send power swap failed")
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if pdState, err := s.GetPDState(ctx, port); err == nil {
				testing.ContextLog(ctx, "PD state after: ", pdState)
				testing.ContextLog(ctx, "PD Role after: ", pdState[0][4])
				nowPowerRole := pdState[0][4]
				if role != nowPowerRole {
					return errors.Wrap(err, "failed to switch power role")
				}
			} else {
				return errors.Wrap(err, "failed to get PD state")
			}

			return nil
		}, &testing.PollOptions{Timeout: pdStatePollTimeout, Interval: pdStatePollInterval}); err != nil {
			return errors.Wrap(err, "expected PD power swap")
		}

	} else {
		testing.ContextLog(ctx, "PD already at power role: ", role)
	}

	return nil
}

// RestorePDPort restores DUT PD port state to the SNK role.
func (s *Servo) RestorePDPort(ctx context.Context, port int) error {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	}

	// Set DUT PD to SNK so battery charges.
	if err := s.SetPDPowerRole(ctx, port, "SNK"); err != nil {
		return errors.Wrap(err, "failed to set PD role to SNK")
	}

	return nil
}
