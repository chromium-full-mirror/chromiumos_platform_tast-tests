// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// A PDPolarityValue defines the CC polarity state
type PDPolarityValue StringControl

// List of polarity values
const (
	PolarityCC1 PDPolarityValue = "CC1"
	PolarityCC2 PDPolarityValue = "CC2"
)

// A PDStatusValue defines the current PD status, enabled or disabled
type PDStatusValue string

// List of PD status values
const (
	PDEnabled  PDStatusValue = "enabled"
	PDDisabled PDStatusValue = "disabled"
)

// A PowerRoleValue defines the current PD port power role
type PowerRoleValue string

// List of PD power roles
const (
	PowerRoleSRC PowerRoleValue = "SRC"
	PowerRoleSNK PowerRoleValue = "SNK"
)

// A DataRoleValue defines the current PD port data role
type DataRoleValue string

// List of PD data roles
const (
	DataRoleDFP DataRoleValue = "DFP"
	DataRoleUFP DataRoleValue = "UFP"
)

// A PDState encapsulates the full PD port state on a servo
type PDState struct {
	Port      int
	Polarity  PDPolarityValue
	Status    PDStatusValue
	PowerRole PowerRoleValue
	DataRole  DataRoleValue
	PEState   int
	Flags     uint32
}

const (
	// ReServoPdStateCommand - Valid for TCPM v1 only
	// Example: Port C1 CC1, Ena - Role: SRC-UFP State: 23(), Flags: 0x1415e
	// Match index:
	//	0 - Full match
	//	1 - Port number		0 or 1
	//	2 - CC Polarity		CC1/CC2
	//	3 - Connection status	Ena/Dis
	//	4 - Power role		SRC/SNK
	//	5 - Data role		DFP/UFP
	//	6 - PE State		number
	//	7 - Flags		32-bit flags
	ReServoPdStateCommand string = `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)\s+State:\s(\d+)\(.*\),\s+Flags:\s+0x(\w*)[\r\n]`
)

// GetServoPDState returns the state of the PD port on the servo that connects to the DUT
// For servoV4 and servoV4p1, the PD port 0 is the charging port and PD port 1 is the DUT port.
func (s *Servo) GetServoPDState(ctx context.Context) (*PDState, error) {
	var portState PDState
	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 state", []string{ReServoPdStateCommand})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get servo PD state")
	}

	testing.ContextLogf(ctx, "Full string: %s", out[0][0])
	testing.ContextLogf(ctx, "Token count : %d", len(out[0]))

	// Port number, for servo it should always be 1
	portState.Port, err = strconv.Atoi(out[0][1])
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert port number from PD state")
	}

	// CC polarity, convert directly to PDPolarityValue
	portState.Polarity = PDPolarityValue(out[0][2])

	// Connection status: Enabled/Disabled
	if strings.HasPrefix(out[0][3], "Ena") {
		portState.Status = PDEnabled
	} else if strings.HasPrefix(out[0][3], "Dis") {
		portState.Status = PDDisabled
	} else {
		return nil, errors.Errorf("invalid PD status: %s", out[0][3])
	}

	// Power role: SRC/SNK
	if strings.HasPrefix(out[0][4], "SRC") {
		portState.PowerRole = PowerRoleSRC
	} else if strings.HasPrefix(out[0][3], "SNK") {
		portState.PowerRole = PowerRoleSNK
	} else {
		return nil, errors.Errorf("invalid power role: %s", out[0][4])
	}

	// Data role: DFP/UFP
	if strings.HasPrefix(out[0][5], "DFP") {
		portState.DataRole = DataRoleDFP
	} else if strings.HasPrefix(out[0][5], "UFP") {
		portState.DataRole = DataRoleUFP
	} else {
		return nil, errors.Errorf("invalid data role: %s", out[0][5])
	}

	// PE state
	portState.PEState, err = strconv.Atoi(out[0][6])
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert PE state number from PD state")
	}

	// Flags
	flags64, err := strconv.ParseUint(out[0][7], 16, 32)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert flags from PD state")
	}
	portState.Flags = uint32(flags64)

	return &portState, nil
}
