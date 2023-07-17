// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	reEcPdStateCommand string = `Port C(\d+) CC(\d+), (\S+) - Role: (\w+)-(\w+) TC State: ([\w\.]+), Flags: 0x([A-Fa-f0-9]+) PE State: (\w+), Flags: 0x([A-Fa-f0-9]+)[\r\n]`
	reEcPdRecv         string = `RECV\s([\w]+)`
	// PdControlMsgMask -- bitmask for PD control messages
	PdControlMsgMask int = 0x1f
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
	testing.ContextLog(ctx, "DualRole reply: ", out)

	return out[0][0] == onResponse, nil
}
