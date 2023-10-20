// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast/core/errors"
)

// pdStateCmdTarget specifies whether pd state commands go to the DUT or Servo
type pdStateCmdTarget int

const (
	pdStateServo pdStateCmdTarget = iota
	pdStateDUT
)

// A pdPolarityValue defines the CC polarity state.
type pdPolarityValue string

// List of polarity values.
const (
	PolarityCC1    pdPolarityValue = "CC1"
	PolarityCC2    pdPolarityValue = "CC2"
	PolarityCC1DTS pdPolarityValue = "CC3"
	PolarityCC2DTS pdPolarityValue = "CC4"
)

// A connectionValue defines the current PD connection status, enabled or disabled.
type connectionValue string

// List of PD status values.
const (
	PDEnabled  connectionValue = "enabled"
	PDDisabled connectionValue = "disabled"
)

// A powerRoleValue defines the current PD port power role.
type powerRoleValue string

// List of PD power roles.
const (
	PowerRoleSRC powerRoleValue = "SRC"
	PowerRoleSNK powerRoleValue = "SNK"
)

// A dataRoleValue defines the current PD port data role.
type dataRoleValue string

// List of PD data roles
const (
	DataRoleDFP dataRoleValue = "DFP"
	DataRoleUFP dataRoleValue = "UFP"
)

// Maps TCPMv1 state numbers to a friendly string name based on EC's `include/usb_pd.h`
var peStateNameLookup = map[int]string{
	0:  "PD_STATE_DISABLED",
	1:  "PD_STATE_SUSPENDED",
	2:  "PD_STATE_SNK_DISCONNECTED",
	3:  "PD_STATE_SNK_DISCONNECTED_DEBOUNCE",
	4:  "PD_STATE_SNK_HARD_RESET_RECOVER",
	5:  "PD_STATE_SNK_DISCOVERY",
	6:  "PD_STATE_SNK_REQUESTED",
	7:  "PD_STATE_SNK_TRANSITION",
	8:  "PD_STATE_SNK_READY",
	9:  "PD_STATE_SNK_SWAP_INIT",
	10: "PD_STATE_SNK_SWAP_SNK_DISABLE",
	11: "PD_STATE_SNK_SWAP_SRC_DISABLE",
	12: "PD_STATE_SNK_SWAP_STANDBY",
	13: "PD_STATE_SNK_SWAP_COMPLETE",
	14: "PD_STATE_SRC_DISCONNECTED",
	15: "PD_STATE_SRC_DISCONNECTED_DEBOUNCE",
	16: "PD_STATE_SRC_HARD_RESET_RECOVER",
	17: "PD_STATE_SRC_STARTUP",
	18: "PD_STATE_SRC_DISCOVERY",
	19: "PD_STATE_SRC_NEGOCIATE",
	20: "PD_STATE_SRC_ACCEPTED",
	21: "PD_STATE_SRC_POWERED",
	22: "PD_STATE_SRC_TRANSITION",
	23: "PD_STATE_SRC_READY",
	24: "PD_STATE_SRC_GET_SINK_CAP",
	25: "PD_STATE_DR_SWAP",
	26: "PD_STATE_SRC_SWAP_INIT",
	27: "PD_STATE_SRC_SWAP_SNK_DISABLE",
	28: "PD_STATE_SRC_SWAP_SRC_DISABLE",
	29: "PD_STATE_SRC_SWAP_STANDBY",
	30: "PD_STATE_VCONN_SWAP_SEND",
	31: "PD_STATE_VCONN_SWAP_INIT",
	32: "PD_STATE_VCONN_SWAP_READY",
	33: "PD_STATE_SOFT_RESET",
	34: "PD_STATE_HARD_RESET_SEND",
	35: "PD_STATE_HARD_RESET_EXECUTE",
	36: "PD_STATE_BIST_RX",
	37: "PD_STATE_BIST_TX",
	38: "PD_STATE_DRP_AUTO_TOGGLE",
}

// pdStateFieldIndex maps field names to their position in the regex. Supports
// output for both TCPM stack versions.
var pdStateFieldIndex = map[TCPMVersion]map[string]int{
	TCPMv1: {
		"Full":       0,
		"PortNumber": 1,
		"CCPolarity": 2,
		"Connection": 3,
		"PowerRole":  4,
		"DataRole":   5,
		"PEState":    6,
		"PEFlags":    7,
	},
	TCPMv2: {
		"Full":       0,
		"PortNumber": 1,
		"CCPolarity": 2,
		"Connection": 3,
		"PowerRole":  4,
		"DataRole":   5,
		"TCState":    6,
		"TCFlags":    7,
		"PEState":    8,
		"PEFlags":    9,
	},
}

// pdStateFieldLookup is used for mapping console output to internal constants
var pdStateFieldLookup = map[string]map[string]string{
	"CCPolarity": {"CC1": string(PolarityCC1), "CC2": string(PolarityCC2),
		"CC3": string(PolarityCC1DTS), "CC4": string(PolarityCC2DTS)},
	"Connection": {"Ena": string(PDEnabled), "Dis": string(PDDisabled),
		"Enable": string(PDEnabled), "Disable": string(PDDisabled)},
	"PowerRole": {"SRC": string(PowerRoleSRC), "SNK": string(PowerRoleSNK)},
	"DataRole":  {"DFP": string(DataRoleDFP), "UFP": string(DataRoleUFP)},
}

var pdStateCmdRegexp = map[TCPMVersion]string{
	// For TCPMv1 DUTs and Servo
	//   Example: "Port C0 CC1, Ena - Role: SNK-DFP State: 8(), Flags: 0x16946"
	//   Match Index:
	//      0 - Full match
	//      1 - Port number  -- 0
	//      2 - Polarity     -- 1
	//      3 - Comm Status  -- Enable
	//      4 - Power role   -- SRC
	//      5 - Data role    -- DFP
	//      6 - PE State     -- 8
	//      7 - PE Flags     -- 16946
	TCPMv1: `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)\s+State:\s+(\d+)\(\w*\),\s+Flags:\s+0x(\w+)[\r\n]`,
	// For TCPMv2 DUTs
	//   Example: "Port C0 CC1, Enable - Role: SNK-DFP TC State: Attached.SNK, Flags: 0x9012 PE State: PE_SNK_Ready, Flags: 0x0201 SPR"
	//   Match Index:
	//      0 - Full match
	//      1 - Port number  -- 0
	//      2 - Polarity     -- 1
	//      3 - Comm Status  -- Enable
	//      4 - Power role   -- SRC
	//      5 - Data role    -- DFP
	//      6 - TC State     -- Attached.SNK (optional)
	//      7 - TC Flags     -- 9012
	//      8 - PE State     -- PE_SNK_Ready
	//      9 - PE Flags     -- 0201
	//     10 - Extra fields -- SPR
	TCPMv2: `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)\s+TC State:\s+([\w\.]+)?,\s+Flags:\s+0x(\w+)\s+PE State:\s+(\w+)?,\s+Flags:\s+0x(\w+)\s+(.*)[\r\n]`,
}

// Helper type that stores raw regex output
type pdStateTokens []string

func (t *pdStateTokens) lookup(field string, ver TCPMVersion) (string, error) {
	token := (*t)[pdStateFieldIndex[ver][field]]
	if value, ok := pdStateFieldLookup[field][token]; ok {
		return value, nil
	}
	return "", errors.Errorf("PD field %q contains unknown value %q", field, token)
}

func (t *pdStateTokens) peStateName(ver TCPMVersion) (string, error) {
	token := (*t)[pdStateFieldIndex[ver]["PEState"]]

	if ver == TCPMv1 {
		// Token is a string integer. Convert and map to state name.
		stateNum, err := strconv.Atoi(token)
		if err != nil {
			return "", errors.Wrap(err, "cannot convert PE state")
		}

		stateName, ok := peStateNameLookup[stateNum]
		if !ok {
			return "", errors.Errorf("unknown PE state %d", stateNum)
		}
		return stateName, nil

	} else if ver == TCPMv2 {
		// Token is already the state name
		return token, nil
	}

	panic("Invalid TCPM version")
}

// PDState encapsulates the full PD port state on an EC or Servo
type PDState struct {
	Version     TCPMVersion
	Port        int
	Polarity    pdPolarityValue
	Connection  connectionValue
	PowerRole   powerRoleValue
	DataRole    dataRoleValue
	PEStateName string
	PEFlags     uint32
	TCStateName string // TCPMv2 DUTs only
	TCFlags     uint32 // TCPMv2 DUTs only
}

// getPDStateByTargetAndVersion queries state for a specific port on either a
// DUT or Servo under a specified TCPM version. Do not call this directly. Use
// GetServoPDState, GetServoChargerPortPDState, or GetDUTPDState.
func (s *Servo) getPDStateByTargetAndVersion(
	ctx context.Context,
	target pdStateCmdTarget,
	ver TCPMVersion,
	port int,
) (*PDState, error) {
	// Because this helper may be run before s.dutPDInfo is populated,
	// require an explicit port.
	if port == PDPortUnderTest {
		panic("This method may only be called with an exact port, not PDPortUnderTest")
	}

	if ver == TCPMv2 && target == pdStateServo {
		panic("Servo does not use TCPMv2. Must pass TCPMv1.")
	}

	// Get the correct regex based on version and build the command
	regex := pdStateCmdRegexp[ver]
	cmd := fmt.Sprintf("pd %d state", port)

	var t pdStateTokens

	if target == pdStateDUT {
		cmdOutput, err := s.RunECCommandGetOutputNoConsoleLogs(ctx, cmd, []string{regex})
		if err != nil {
			return nil, errors.Wrapf(err, "EC command %q failed", cmd)
		}
		t = pdStateTokens(cmdOutput[0])
	} else if target == pdStateServo {
		// Run command on the servo console
		cmdOutput, err := s.RunServoCommandGetOutput(ctx, cmd, []string{regex})
		if err != nil {
			return nil, errors.Wrapf(err, "Servo command %q failed", cmd)
		}
		t = pdStateTokens(cmdOutput[0])
	} else {
		panic("Invalid target to send command to")
	}

	var err error
	var portState PDState

	//
	// Fill in fields common to both TCPM versions
	//

	portState.Version = ver

	portState.Port, err = strconv.Atoi(t[pdStateFieldIndex[ver]["PortNumber"]])
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert port number")
	}

	polarity, err := t.lookup("CCPolarity", ver)
	if err != nil {
		return nil, err
	}
	portState.Polarity = pdPolarityValue(polarity)

	connection, err := t.lookup("Connection", ver)
	if err != nil {
		return nil, err
	}
	portState.Connection = connectionValue(connection)

	powerRole, err := t.lookup("PowerRole", ver)
	if err != nil {
		return nil, err
	}
	portState.PowerRole = powerRoleValue(powerRole)

	dataRole, err := t.lookup("DataRole", ver)
	if err != nil {
		return nil, err
	}
	portState.DataRole = dataRoleValue(dataRole)

	//
	// PE state and flags
	//

	portState.PEStateName, err = t.peStateName(ver)
	if err != nil {
		return nil, err
	}

	flags64, err := strconv.ParseUint(t[pdStateFieldIndex[ver]["PEFlags"]], 16, 32)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert PE flags number")
	}
	portState.PEFlags = uint32(flags64)

	//
	// TC state and flags (TCPMv2 only)
	//

	if ver == TCPMv2 {
		portState.TCStateName = t[pdStateFieldIndex[ver]["TCState"]]

		flags64, err := strconv.ParseUint(
			t[pdStateFieldIndex[ver]["TCFlags"]], 16, 32,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert TC flags number")
		}
		portState.TCFlags = uint32(flags64)
	}

	return &portState, nil
}

// GetServoPDState returns the state of the PD port on the servo that connects to the DUT (C1)
func (s *Servo) GetServoPDState(ctx context.Context) (*PDState, error) {
	return s.getPDStateByTargetAndVersion(ctx, pdStateServo, TCPMv1, 1)
}

// GetServoChargerPortPDState returns the state of the PD port on the servo that connects
// to the charger (C0)
func (s *Servo) GetServoChargerPortPDState(ctx context.Context) (*PDState, error) {
	return s.getPDStateByTargetAndVersion(ctx, pdStateServo, TCPMv1, 0)
}

// GetDUTPDState returns PD state info for the PD port on the EC/DUT.
func (s *Servo) GetDUTPDState(ctx context.Context) (*PDState, error) {
	// This function requires TCPM version info discovered by Servo.RequireDUTPDInfo()
	if err := s.RequireDUTPDInfo(ctx); err != nil {
		return nil, errors.Wrap(err, "cannot discover DUT PD info")
	}
	return s.getPDStateByTargetAndVersion(ctx, pdStateDUT, s.dutPDInfo.version, s.dutPDInfo.activePort)
}
