// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"strings"

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
	// PdControlMsgMask -- bitmask for PD control messages
	PdControlMsgMask int = 0x1f
	// MaxPorts -- Max number of ports on EC
	MaxPorts int = 4
)

// GetPDState Returns PD state console output
func (s *Servo) GetPDState(ctx context.Context, port int) ([][]string, error) {
	cmd := fmt.Sprintf("pd %d state", port)

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{reEcPdStateCommand})
	if err != nil {
		return nil, errors.Wrap(err, "EC pd command failed")
	}

	return out, nil
}

// SendPowerSwapRequest sends power swap request
func (s *Servo) SendPowerSwapRequest(ctx context.Context, port int) error {
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
	cmd := fmt.Sprintf("pd %d dualrole", port)
	onResponse := "on"

	out, err := s.RunECCommandGetOutput(ctx, cmd, []string{`dual-role toggling:\s+([\w ]+)[\r\n]`})

	if err != nil {
		return false, errors.Wrap(err, "EC pd command failed")
	}
	testing.ContextLog(ctx, "DualRole reply: ", out[0][1])

	return out[0][1] == onResponse, nil
}

// GetPdPort returns enabled PD port number on EC
func (s *Servo) GetPdPort(ctx context.Context) (int, error) {

	pdPort := MaxPorts
	numFound := 0

	for port := 0; port < MaxPorts; port++ {
		if out, err := s.GetPDState(ctx, port); err == nil {
			testing.ContextLog(ctx, "PD state out[0][3]: ", out[0][3])
			if strings.HasPrefix(out[0][3], "Ena") {
				pdPort = port
				numFound++
			}
		}
	}

	if numFound == 0 {
		return pdPort, errors.New("no PD ports found")
	} else if numFound > 1 {
		return pdPort, errors.New("more than one PD port found")
	}
	testing.ContextLog(ctx, "Found PD port: ", pdPort)

	return pdPort, nil
}
