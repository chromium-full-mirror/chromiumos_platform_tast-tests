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
	rePDVersion string = `[\r\n](\d|Wrong)`
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
	PDC    TCPMVersion = 3
)

// Supported DualRole states
const (
	DROn     string = `on`
	DROff    string = `off`
	DRFreeze string = `freeze`
	DRSink   string = `sink`
	DRSource string = `source`
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
		case 3:
			pdInfo.version = PDC
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

			if portInfo.IsPDReady() {
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

// DUTPDPort returns the active PD port on the DUT.
func (s *Servo) DUTPDPort() int {
	return s.dutPDInfo.activePort
}

const (
	pdStatePollTimeout  time.Duration = 20 * time.Second
	pdStatePollInterval time.Duration = 500 * time.Millisecond
)

// SendRequestSourceVoltage sends a voltage request to the source that's initiated by the DUT.
func (s *Servo) SendRequestSourceVoltage(ctx context.Context, voltage int) error {
	cmd := fmt.Sprintf("pd %d dev %d", s.dutPDInfo.activePort, voltage)

	testing.ContextLog(ctx, "Sending request source voltage: ", cmd)
	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// SetDualroleState sets the dual-role state of the DUT
func (s *Servo) SetDualroleState(ctx context.Context, drs string) error {
	cmd := fmt.Sprintf("pd %d dualrole %s", s.dutPDInfo.activePort, drs)

	testing.ContextLog(ctx, "Sending request: ", cmd)
	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// SendPowerSwapRequest sends power swap request to be initiated by the DUT.
func (s *Servo) SendPowerSwapRequest(ctx context.Context) error {
	cmd := fmt.Sprintf("pd %d swap power", s.dutPDInfo.activePort)

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
func (s *Servo) SendDataSwapRequest(ctx context.Context) error {
	cmd := fmt.Sprintf("pd %d swap data", s.dutPDInfo.activePort)

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

	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// DisablePDConsoleDebug disables PD console debugging on the DUT.
func (s *Servo) DisablePDConsoleDebug(ctx context.Context) error {
	cmd := "pd dump 0"

	if err := s.RunECCommand(ctx, cmd); err != nil {
		return errors.Wrap(err, "EC pd command failed")
	}

	return nil
}

// SetPDPowerRole sets the PD power role for a PD port on the DUT.
func (s *Servo) SetPDPowerRole(ctx context.Context, role string) error {
	pdState, err := s.GetDUTPDState(ctx)

	if err != nil {
		return errors.Wrap(err, "failed to get PD State")
	}

	if string(pdState.PowerRole) != role {
		if err := s.SendPowerSwapRequest(ctx); err != nil {
			return errors.Wrap(err, "send power swap failed")
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if pdState, err := s.GetDUTPDState(ctx); err == nil {
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
func (s *Servo) RestorePDPort(ctx context.Context) error {
	// Set DUT PD to SNK so battery charges.
	if err := s.SetPDPowerRole(ctx, "SNK"); err != nil {
		return errors.Wrap(err, "failed to set PD role to SNK")
	}

	return nil
}

// TriggerPDSoftReset triggers a USB-PD Soft Reset from the EC/DUT-side
func (s *Servo) TriggerPDSoftReset(ctx context.Context) error {
	// Get port status
	pdStateBefore, err := s.GetDUTPDState(ctx)
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

	// Poll until the pre- and post-reset states match or we time out.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdStateAfter, err := s.GetDUTPDState(ctx)
		if err != nil {
			return errors.Wrap(err, "cannot read DUT PD state")
		}

		return pdStateBefore.Compare(pdStateAfter)
	}, &testing.PollOptions{Timeout: pdStatePollTimeout, Interval: pdStatePollInterval}); err != nil {
		return errors.Wrap(err, "timed out waiting for states to match after DUT soft reset")
	}

	// TODO (b/317808083) query the servo's soft reset counter here

	return nil
}

// TriggerPDHardReset triggers a USB-PD Hard Reset from the EC/DUT-side
func (s *Servo) TriggerPDHardReset(ctx context.Context) error {
	var pdStateBefore *PDState
	// Because the pass criteria expects the PE state to be the same after
	// the hard reset as before, make sure the PE/PD state is in either
	// the SNK_READY or SRC_READY state
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if pdStateBefore, err = s.GetDUTPDState(ctx); err == nil {
			if !pdStateBefore.IsPDReady() {
				testing.ContextLogf(ctx, "PD State = %s", pdStateBefore.PEStateName)
				return errors.Wrap(err, "Post hard reset, PE state does not match")
			}
		} else {
			return errors.Wrap(err, "failed to get PD state")
		}
		return nil
	}, &testing.PollOptions{Timeout: pdStatePollTimeout, Interval: pdStatePollInterval}); err != nil {
		return errors.Wrap(err, "DUT port is not in Ready state")
	}

	testing.ContextLogf(ctx, "Before status: %s/%s %s", pdStateBefore.PowerRole, pdStateBefore.DataRole, pdStateBefore.PEStateName)

	if err := s.EnablePDConsoleDebug(ctx); err != nil {
		return errors.Wrap(err, "could not enable EC/DUT's PD debug logs")
	}

	// Initiate hard reset from DUT
	err := s.RunECCommand(
		ctx,
		fmt.Sprintf("pd %d hard", s.dutPDInfo.activePort),
	)
	if err != nil {
		return errors.Wrap(err, "could not trigger hard reset on EC/DUT")
	}

	// Hard reset should result in the DUT port being in the same power/data
	// role and PE state it was in prior to the hard reset being initiated.
	// Poll for this condition, if not reached, then return an error
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if pdState, err := s.GetDUTPDState(ctx); err == nil {
			testing.ContextLogf(ctx, "After status: %s/%s %s", pdState.PowerRole, pdState.DataRole, pdState.PEStateName)
			if pdState.PEStateName != pdStateBefore.PEStateName {
				return errors.Wrap(err, "Post hard reset, PE state does not match")
			}
			if pdState.DataRole != pdStateBefore.DataRole {
				return errors.Wrap(err, "Post hard reset, data role does not match")
			}
			if pdState.PowerRole != pdStateBefore.PowerRole {
				return errors.Wrap(err, "Post hard reset, power role does not match")
			}
		} else {
			return errors.Wrap(err, "failed to get PD state")
		}

		return nil
	}, &testing.PollOptions{Timeout: pdStatePollTimeout, Interval: pdStatePollInterval}); err != nil {
		return errors.Wrap(err, "Post Hard Reset connection does not match before")
	}

	// Go back to `pd dump 0` after.
	if err := s.DisablePDConsoleDebug(ctx); err != nil {
		return errors.Wrap(err, "could not disable EC/DUT's PD debug logs")
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

// GetDUTDualRoleState accepts a port ID and checks for the PD DRP status of this port.
func (s *Servo) GetDUTDualRoleState(ctx context.Context, port int) (USBPdDualRoleValue, error) {
	if port == PDPortUnderTest {
		port = s.dutPDInfo.activePort
	}

	matchList := []string{`dual-role toggling:\s+([\w ]+)[\r\n]`}

	// Try modern `pd N dualrole` command
	cmd := fmt.Sprintf("pd %d dualrole", port)
	out, err := s.RunECCommandGetOutputNoConsoleLogs(ctx, cmd, matchList)
	if err != nil {
		testing.ContextLogf(
			ctx, "EC command %q failed. Trying older version. (%q)",
			cmd, err,
		)
	} else {
		goto success
	}

	// Older DUTs running firmware from before cl:1096654 don't have per-port
	// dualrole settings. Fall back to the old command.
	out, err = s.RunECCommandGetOutputNoConsoleLogs(ctx, "pd dualrole", matchList)
	if err != nil {
		// DUT does not support DRP
		return "", errors.Wrapf(err, "ec command %q failed. No way to check dual role state", cmd)
	}

success:
	testing.ContextLogf(ctx, "Port %d DRP status: %q", port, out[0][1])
	return USBPdDualRoleValue(out[0][1]), nil
}
