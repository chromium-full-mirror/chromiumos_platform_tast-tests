// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"strconv"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// A PDPolarityValue defines the CC polarity state
type PDPolarityValue string

// List of polarity values
const (
	PolarityCC1 PDPolarityValue = "CC1"
	PolarityCC2 PDPolarityValue = "CC2"
)

// A ConnectionValue defines the current PD connection status, enabled or disabled
type ConnectionValue string

// List of PD status values
const (
	PDEnabled  ConnectionValue = "enabled"
	PDDisabled ConnectionValue = "disabled"
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

type servoStateTokens []string

func (t *servoStateTokens) lookup(field string) (string, error) {
	token := (*t)[pdFieldIndex[field]]
	if value, ok := pdFieldLookup[field][token]; ok {
		return value, nil
	}
	return "", errors.Errorf("PD field %q contains unknown value %q", field, token)
}

// A PDState encapsulates the full PD port state on a servo
type PDState struct {
	Port       int
	Polarity   PDPolarityValue
	Connection ConnectionValue
	PowerRole  PowerRoleValue
	DataRole   DataRoleValue
	PEState    int
	Flags      uint32
}

const (
	// ReServoPdStateCommand - Valid for TCPM v1 only
	// Example: Port C1 CC1, Ena - Role: SRC-UFP State: 23(), Flags: 0x1415e
	ReServoPdStateCommand string = `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)\s+State:\s(\d+)\(.*\),\s+Flags:\s+0x(\w*)[\r\n]`
)

var pdFieldIndex = map[string]int{
	"Full":       0,
	"PortNumber": 1,
	"CCPolarity": 2,
	"Connection": 3,
	"PowerRole":  4,
	"DataRole":   5,
	"PEState":    6,
	"Flags":      7,
}

// PD fields lookup.  Primary key must match a key from pdFieldIndex
// Sub-keys match the servo output from the "pd state" command
var pdFieldLookup = map[string]map[string]string{
	"CCPolarity": {"CC1": string(PolarityCC1), "CC2": string(PolarityCC2)},
	"Connection": {"Ena": string(PDEnabled), "Dis": string(PDDisabled)},
	"PowerRole":  {"SRC": string(PowerRoleSRC), "SNK": string(PowerRoleSNK)},
	"DataRole":   {"DFP": string(DataRoleDFP), "UFP": string(DataRoleUFP)},
}

// GetServoPDState returns the state of the PD port on the servo that connects to the DUT
// For servoV4 and servoV4p1, the PD port 0 is the charging port and PD port 1 is the DUT port.
func (s *Servo) GetServoPDState(ctx context.Context) (*PDState, error) {
	var portState PDState
	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 state", []string{ReServoPdStateCommand})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get servo PD state")
	}

	t := servoStateTokens(out[0])

	testing.ContextLogf(ctx, "Full string: %s", t[pdFieldIndex["Full"]])
	testing.ContextLogf(ctx, "Token count : %d", len(t))

	portState.Port, err = strconv.Atoi(t[pdFieldIndex["PortNumber"]])
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert port number")
	}

	polarity, err := t.lookup("CCPolarity")
	if err != nil {
		return nil, err
	}
	portState.Polarity = PDPolarityValue(polarity)

	connection, err := t.lookup("Connection")
	if err != nil {
		return nil, err
	}
	portState.Connection = ConnectionValue(connection)

	powerRole, err := t.lookup("PowerRole")
	if err != nil {
		return nil, err
	}
	portState.PowerRole = PowerRoleValue(powerRole)

	dataRole, err := t.lookup("DataRole")
	if err != nil {
		return nil, err
	}
	portState.DataRole = DataRoleValue(dataRole)

	portState.PEState, err = strconv.Atoi(t[pdFieldIndex["PEState"]])
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert PE state number")
	}

	flags64, err := strconv.ParseUint(t[pdFieldIndex["Flags"]], 16, 32)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert PE state number")
	}
	portState.Flags = uint32(flags64)

	return &portState, nil
}
