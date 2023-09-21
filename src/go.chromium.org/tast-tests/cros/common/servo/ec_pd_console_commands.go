// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// reEcPdStateCommand -- TCPM v1 and v2 compatible regex for pd <port> state output
	//   Example: Port C0 CC3, Enable - Role: SRC-DFP TC State: Attached.SRC, Flags: 0x9002 PE State: PE_SRC_Ready, Flags: 0x0201
	//   Match Index:
	//      0 - Full match
	//      1 - Port number  -- 0
	//      2 - Polarity     -- 3
	//      3 - Comm Status  -- Enable
	//      4 - Power role   -- SRC
	//      5 - Data role    -- DFP
	reEcPdStateCommand string = `Port\s+C(\d+)\s+CC(\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)(.*)[\r\n]`
	reEcPdRecv         string = `RECV\s([\w]+)`
	rePDVersion        string = `\s+(\d+|Wrong.*)`
	// PdControlMsgMask -- bitmask for PD control messages
	PdControlMsgMask int = 0x1f
	// MaxPorts -- Max number of ports on EC
	MaxPorts int = 4
	// PDPortUnderTest indicates command should be sent to the PD port conntected to servo
	PDPortUnderTest int = MaxPorts
)

// DUTPDInfo caches the fixed PD testing information for the DUT
type DUTPDInfo struct {
	version    int // 1==TCPMv1, 2==TCPMv1
	activePort int // PD port connected to servo
	portCount  int // Total number of PD ports on the DUT
}

// RequireDUTPDInfo allocates and caches the fixed information about the PD port under test
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

	numPorts := 0
	enabledPorts := 0
	pdPort := MaxPorts
	for port := 0; port < MaxPorts; port++ {
		if out, err := s.GetPDState(ctx, port); err == nil {
			testing.ContextLog(ctx, "PD state out[0][3]: ", out[0][3])
			if strings.HasPrefix(out[0][3], "Ena") {
				pdPort = port
				enabledPorts++
			}
			numPorts++
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

// GetPDState Returns PD state console output
func (s *Servo) GetPDState(ctx context.Context, port int) ([][]string, error) {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
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

// SendPowerSwapRequest sends power swap request
func (s *Servo) SendPowerSwapRequest(ctx context.Context, port int) error {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	}
	cmd := fmt.Sprintf("pd %d swap power", port)

	s.EnablePDConsoleDebug(ctx)

	testing.ContextLog(ctx, "Sending power swap request: ", cmd)

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{reEcPdRecv})

	if err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}
	testing.ContextLog(ctx, "PowerSwap reply: ", out)

	s.DisablePDConsoleDebug(ctx)

	return nil
}

// EnablePDConsoleDebug enables PD console debugging level 2
func (s *Servo) EnablePDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 2"

	testing.ContextLog(ctx, "Enabling PD Console Debug")
	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// DisablePDConsoleDebug disables PD console debugging
func (s *Servo) DisablePDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 0"

	testing.ContextLog(ctx, "Disabling PD Console Debug")
	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// GetDualRole returns true if dual role is enabled on port
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

// SetPDPowerRole - Sets PD power role
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

// RestorePDPort - Restores DUT PD port
func (s *Servo) RestorePDPort(ctx context.Context, port int) error {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	}

	// Set DUT PD to SNK so battery charges
	if err := s.SetPDPowerRole(ctx, port, "SNK"); err != nil {
		return errors.Wrap(err, "failed to set PD role to SNK")
	}

	return nil
}
