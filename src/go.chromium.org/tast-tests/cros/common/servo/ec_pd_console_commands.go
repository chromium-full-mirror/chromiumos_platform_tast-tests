// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	reEcPdRecv  string = `RECV\s([\w]+)`
	rePDVersion string = `\s+(\d+|Wrong.*)`
	// MaxPorts specifies the maximum number of ports on the EC.
	MaxPorts int = 4
	// PDPortUnderTest indicates command should be sent to the PD port connected to servo.
	PDPortUnderTest int = MaxPorts
)

// TCPMVersion is a type for denoting a TCPM stack version
type TCPMVersion int

// Supported TCPM Versions
const (
	TCPMv1 TCPMVersion = 1
	TCPMv2 TCPMVersion = 2
)

// DUTPDInfo caches the fixed PD testing information for the DUT.
type DUTPDInfo struct {
	version    TCPMVersion // TCPM stack version in use by DUT
	activePort int         // PD port connected to servo
	portCount  int         // Total number of PD ports on the DUT
}

// RequireDUTPDInfo allocates and caches the fixed information about the PD port under test.
func (s *Servo) RequireDUTPDInfo(ctx context.Context) error {
	if s.dutPDInfo != nil {
		return nil
	}

	pdInfo := &DUTPDInfo{}

	out, err := s.RunECCommandGetOutputNoConsoleLogs(ctx, "pd version", []string{rePDVersion})
	if err != nil {
		return errors.Wrap(err, "EC pd version failed")
	}

	if ver, err := strconv.Atoi(out[0][1]); err != nil {
		testing.ContextLog(
			ctx,
			"PD Version command is not supported. This test is likely running "+
				"against an old version of the EC. The test will assume the "+
				" DUT is running the TCPMv1 stack. This may cause errors.",
		)
		pdInfo.version = TCPMv1
	} else {
		switch ver {
		case 1:
			pdInfo.version = TCPMv1
		case 2:
			pdInfo.version = TCPMv2
		default:
			return errors.Errorf("invalid TCPM version (%d) Output: %q", ver, out)
		}
	}

	numPorts := 0
	enabledPorts := 0
	pdPort := MaxPorts
	for port := 0; port < MaxPorts; port++ {
		if portInfo, err := s.getPDStateByTargetAndVersion(ctx, pdStateDUT, pdInfo.version, port); err == nil {
			testing.ContextLogf(ctx, "DUT Port %d state: %#v", port, portInfo)

			var activePEStates = map[string]bool{
				"PD_STATE_SNK_READY": true,
				"PD_STATE_SRC_READY": true,
				"PE_SNK_Ready":       true,
				"PE_SRC_Ready":       true,
			}

			if _, ok := activePEStates[portInfo.PEStateName]; ok {
				pdPort = port
				enabledPorts++
			}
			numPorts++
		} else {
			testing.ContextLogf(ctx, "DUT Port %d not present (%q)", port, err)
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
	pdState, err := s.GetDUTPDState(ctx, port)

	if err != nil {
		return errors.Wrap(err, "failed to get PD State")
	}

	if string(pdState.PowerRole) != role {
		if err := s.SendPowerSwapRequest(ctx, port); err != nil {
			return errors.Wrap(err, "send power swap failed")
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if pdState, err := s.GetDUTPDState(ctx, port); err == nil {
				testing.ContextLogf(ctx, "PD state after: %#v", pdState)
				testing.ContextLog(ctx, "PD Role after: ", pdState.PowerRole)
				if role != string(pdState.PowerRole) {
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

// TriggerPDSoftReset triggers a USB-PD Soft Reset from the EC/DUT-side
func (s *Servo) TriggerPDSoftReset(ctx context.Context) error {
	// Get port status
	pdStateBefore, err := s.GetDUTPDState(ctx, PDPortUnderTest)
	if err != nil {
		return errors.Wrap(err, "failed to get pre-test EC/DUT-side PD port status")
	}

	if err := s.EnablePDConsoleDebug(ctx); err != nil {
		return errors.Wrap(err, "could not enable EC/DUT's PD debug logs")
	}

	// Run the command
	err = s.RunECCommand(
		ctx,
		fmt.Sprintf("pd %d soft", s.dutPDInfo.activePort),
	)
	if err != nil {
		return errors.Wrap(err, "could not trigger soft reset on EC/DUT")
	}

	// Go back to `pd dump 0` after.
	if err := s.DisablePDConsoleDebug(ctx); err != nil {
		return errors.Wrap(err, "could not disable EC/DUT's PD debug logs")
	}

	// Compare PD state before and after (should be the same)
	pdStateAfter, err := s.GetDUTPDState(ctx, PDPortUnderTest)
	if err != nil {
		return errors.Wrap(err, "failed to get post-test EC/DUT-side PD port status")
	}

	// Connection status
	if pdStateBefore.Connection != pdStateAfter.Connection {
		return errors.Errorf(
			"PD connection state changed after soft reset. Now %s, expected %s",
			pdStateAfter.Connection,
			pdStateBefore.Connection,
		)
	}

	// Power role
	if pdStateBefore.PowerRole != pdStateAfter.PowerRole {
		return errors.Errorf(
			"Power role changed after soft reset. Now %s, expected %s",
			pdStateAfter.PowerRole,
			pdStateBefore.PowerRole,
		)
	}

	// Data role
	if pdStateBefore.DataRole != pdStateAfter.DataRole {
		return errors.Errorf(
			"Data role changed after soft reset. Now %s, expected %s",
			pdStateAfter.DataRole,
			pdStateBefore.DataRole,
		)
	}

	return nil
}

// SaveDUTConsoleChannelMask stores the current console channel mask on the DUT
func (s *Servo) SaveDUTConsoleChannelMask(ctx context.Context) error {
	return s.RunECCommand(ctx, "chan save")
}

// RestoreDUTConsoleChannelMask stores the current console channel mask on the DUT
func (s *Servo) RestoreDUTConsoleChannelMask(ctx context.Context) error {
	return s.RunECCommand(ctx, "chan restore")
}

// SetDUTConsoleChannelMask sets a give console channel mask on the EC
func (s *Servo) SetDUTConsoleChannelMask(ctx context.Context, mask uint32) error {
	cmd := fmt.Sprintf("chan %08x", mask)

	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrapf(err, "could not set EC chan mask to 0x%08x", mask)
	}

	return nil
}
