// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"regexp"
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
	0:  "DISABLED",
	1:  "SUSPENDED",
	2:  "SNK_DISCONNECTED",
	3:  "SNK_DISCONNECTED_DEBOUNCE",
	4:  "SNK_HARD_RESET_RECOVER",
	5:  "SNK_DISCOVERY",
	6:  "SNK_REQUESTED",
	7:  "SNK_TRANSITION",
	8:  "SNK_READY",
	9:  "SNK_SWAP_INIT",
	10: "SNK_SWAP_SNK_DISABLE",
	11: "SNK_SWAP_SRC_DISABLE",
	12: "SNK_SWAP_STANDBY",
	13: "SNK_SWAP_COMPLETE",
	14: "SRC_DISCONNECTED",
	15: "SRC_DISCONNECTED_DEBOUNCE",
	16: "SRC_HARD_RESET_RECOVER",
	17: "SRC_STARTUP",
	18: "SRC_DISCOVERY",
	19: "SRC_NEGOCIATE",
	20: "SRC_ACCEPTED",
	21: "SRC_POWERED",
	22: "SRC_TRANSITION",
	23: "SRC_READY",
	24: "SRC_GET_SNK_CAP",
	25: "DR_SWAP",
	26: "SRC_SWAP_INIT",
	27: "SRC_SWAP_SNK_DISABLE",
	28: "SRC_SWAP_SRC_DISABLE",
	29: "SRC_SWAP_STANDBY",
	30: "VCONN_SWAP_SEND",
	31: "VCONN_SWAP_INIT",
	32: "VCONN_SWAP_READY",
	33: "SOFT_RESET",
	34: "HARD_RESET_SEND",
	35: "HARD_RESET_EXECUTE",
	36: "BIST_RX",
	37: "BIST_TX",
	38: "DRP_AUTO_TOGGLE",
}

// pdStateFieldIndex maps field names to their position in the regex. Supports
// output for both TCPM stack versions plus PDC.
var pdStateFieldIndex = map[TCPMVersion]map[string]int{
	TCPMv1: {
		"Full":       0,
		"PortNumber": 1,
		"CCPolarity": 2,
		"Connection": 3,
		"PowerRole":  4,
		"DataRole":   5,
		"VConn":      6,
		"PEState":    7,
		"PEFlags":    8,
	},
	TCPMv2: {
		"Full":       0,
		"PortNumber": 1,
		"CCPolarity": 2,
		"Connection": 3,
		"PowerRole":  4,
		"DataRole":   5,
		"VConn":      6,
		"TCState":    7,
		"TCFlags":    8,
		"PEState":    9,
		"PEFlags":    10,
	},
	PDC: {
		"Full":       0,
		"PortNumber": 1,
		"CCPolarity": 2,
		"Connection": 3,
		"PowerRole":  4,
		"DataRole":   5,
		"Vconn":      6,
		"PDCState":   7,
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
	//      6 - VConn (optional "-VC")
	//      7 - PE State     -- 8
	//      8 - PE Flags     -- 16946
	TCPMv1: `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)(-VC)?\s+State:\s+(\S+),\s+Flags:\s+0x(\w+)[\r\n]`,
	// For TCPMv2 DUTs
	//   Example: "Port C0 CC1, Enable - Role: SNK-DFP TC State: Attached.SNK, Flags: 0x9012 PE State: PE_SNK_Ready, Flags: 0x0201 SPR"
	//   Match Index:
	//      0 - Full match
	//      1 - Port number  -- 0
	//      2 - Polarity     -- 1
	//      3 - Comm Status  -- Enable
	//      4 - Power role   -- SRC
	//      5 - Data role    -- DFP
	//      6 - VConn (optional "-VC")
	//      7 - TC State     -- Attached.SNK (optional)
	//      8 - TC Flags     -- 9012
	//      9 - PE State     -- PE_SNK_Ready
	//     10 - PE Flags     -- 0201
	//     11 - Extra fields -- SPR
	TCPMv2: `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)(-VC)?\s+TC State:\s+([\w\.]+)?,\s+Flags:\s+0x(\w+)\s+PE State:\s+(\w+)?,\s+Flags:\s+0x(\w+)\s+(.*)[\r\n]`,
	// For PDC DUTs
	//   Example: "C1 CC1, Enable - Role: SNK-DFP PDC State: Attached.SNK"
	//   Match Index:
	//      0 - Full match
	//      1 - Port number  -- 1
	//      2 - CC Polarity  -- CC1
	//      3 - Comm Status  -- Enable
	//      4 - Power role   -- SNK
	//      5 - Data role    -- DFP
	//      6 - VConn (optional "-VC")
	//      7 - PDC State    -- Attached.SNK (See `pdc_state_names` in the EC's pdc_power_mgmt.c)
	PDC: `C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)(-VC)?\s+PDC State:\s+([\w\. ]+)?[\r\n]`,
}

const pdStateInvalidPortRegexp string = `Parameter (\d+) invalid`

// Helper type that stores raw regex output
type pdStateTokens []string

func (t *pdStateTokens) lookup(field string, ver TCPMVersion) (string, error) {
	token := (*t)[pdStateFieldIndex[ver][field]]
	if value, ok := pdStateFieldLookup[field][token]; ok {
		return value, nil
	}
	return "", errors.Errorf("PD field %q contains unknown value %q", field, token)
}

var peStateNameRe = regexp.MustCompile(`(\d+)\((\w*)\)`)

func (t *pdStateTokens) peStateName(ver TCPMVersion) (string, error) {
	token := (*t)[pdStateFieldIndex[ver]["PEState"]]

	if ver == TCPMv1 {
		// Token may be a string, or a number followed by a string in () or just a number. Convert and map to state name.
		m := peStateNameRe.FindStringSubmatch(token)
		if len(m) > 2 && m[2] != "" { // State name in parens
			return m[2], nil
		}
		if len(m) > 1 && m[1] != "" { // Integer before parens, but nothing in the parens
			stateNum, err := strconv.Atoi(m[1])
			if err != nil {
				return "", errors.Wrap(err, "cannot convert PE state")
			}

			stateName, ok := peStateNameLookup[stateNum]
			if !ok {
				return "", errors.Errorf("unknown PE state %d", stateNum)
			}
			return stateName, nil
		}
		// Didn't match the regex, must be a state name.
		return token, nil
	} else if ver == TCPMv2 {
		// Token is already the state name
		return token, nil
	}

	panic("Only TCPMv1 or TCPMv2 supported")
}

// PDState encapsulates the full PD port state on an EC or Servo
type PDState struct {
	Version    TCPMVersion
	Port       int
	Polarity   pdPolarityValue
	Connection connectionValue
	PowerRole  powerRoleValue
	DataRole   dataRoleValue
	VConn      bool

	// Warning: PEStateName and PEFlags have different values and meanings
	// depending on TCPM version. Avoid accessing these members directly
	// and create a method such as IsSourceReady() to obtain the desired
	// info in a version-safe manner, referencing `.Version` if necessary.
	// Not used in PDC DUTs.
	PEStateName string
	PEFlags     uint32

	TCStateName string // TCPMv2 DUTs only
	TCFlags     uint32 // TCPMv2 DUTs only

	PDCState string // PDC DUTs only
}

// IsSourceReady returns true if port is in a source-ready state
func (pdState *PDState) IsSourceReady() bool {
	switch pdState.Version {
	case TCPMv1:
		return pdState.PEStateName == "SRC_READY"
	case TCPMv2:
		return pdState.PEStateName == "PE_SRC_Ready"
	case PDC:
		return pdState.PDCState == "Attached.SRC"
	}
	panic("Unknown TCPM ver")
}

// IsSinkReady returns true if port is in a sink-ready state
func (pdState *PDState) IsSinkReady() bool {
	switch pdState.Version {
	case TCPMv1:
		return pdState.PEStateName == "SNK_READY"
	case TCPMv2:
		return pdState.PEStateName == "PE_SNK_Ready"
	case PDC:
		return pdState.PDCState == "Attached.SNK"
	}
	panic("Unknown TCPM ver")
}

// IsPDReady returns true if the port is in a source- or sink-ready state
func (pdState *PDState) IsPDReady() bool {
	return pdState.IsSourceReady() || pdState.IsSinkReady()
}

// Compare verifies that the connection state, power role, and data role between two
// PDState objects is equivalent and returns nil if so, or an error message.
func (pdState *PDState) Compare(pdStateAfter *PDState) error {
	// Connection status
	if pdState.Connection != pdStateAfter.Connection {
		return errors.Errorf(
			"PD connection state doesn't match. Now %s, was %s",
			pdStateAfter.Connection,
			pdState.Connection,
		)
	}

	// Power role
	if pdState.PowerRole != pdStateAfter.PowerRole {
		return errors.Errorf(
			"Power role doesn't match. Now %s, was %s",
			pdStateAfter.PowerRole,
			pdState.PowerRole,
		)
	}

	// Data role
	if pdState.DataRole != pdStateAfter.DataRole {
		return errors.Errorf(
			"Data role doesn't match. Now %s, was %s",
			pdStateAfter.DataRole,
			pdState.DataRole,
		)
	}

	return nil
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

	if target == pdStateServo && ver != TCPMv1 {
		panic("Servo only supports TCPMv1.")
	}

	// Get the correct regex based on version and build the command
	regex := pdStateCmdRegexp[ver] + "|" + pdStateInvalidPortRegexp

	var cmd string

	// Build the correct command string based on TCPM vs PDC
	switch ver {
	case TCPMv1:
		fallthrough
	case TCPMv2:
		cmd = fmt.Sprintf("pd %d state", port)
	case PDC:
		cmd = fmt.Sprintf("pdc status %d", port)
	default:
		panic("Unknown TCPM version")
	}

	var t pdStateTokens

	if target == pdStateDUT {
		cmdOutput, err := s.RunECCommandGetOutputNoConsoleLogs(ctx, cmd, []string{regex})
		if err != nil {
			return nil, errors.Wrapf(err, "EC command %q failed", cmd)
		}
		invalidPort, _ := regexp.MatchString(pdStateInvalidPortRegexp, cmdOutput[0][0])
		if invalidPort {
			return nil, errors.Errorf("invalid PD port %d", port)
		}
		t = pdStateTokens(cmdOutput[0])
	} else if target == pdStateServo {
		if err := s.RunServoCommand(ctx, "chan save"); err != nil {
			return nil, errors.Wrap(err, "servo console command failed")
		}
		if err := s.RunServoCommand(ctx, "chan 0"); err != nil {
			return nil, errors.Wrap(err, "servo console command failed")
		}
		defer s.RunServoCommand(ctx, "chan restore")
		// Run command on the servo console
		cmdOutput, err := s.RunServoCommandGetOutput(ctx, cmd, []string{regex})
		if err != nil {
			return nil, errors.Wrapf(err, "Servo command %q failed", cmd)
		}
		invalidPort, _ := regexp.MatchString(pdStateInvalidPortRegexp, cmdOutput[0][0])
		if invalidPort {
			return nil, errors.Errorf("invalid PD port %d", port)
		}
		t = pdStateTokens(cmdOutput[0])
	} else {
		panic("Invalid target to send command to")
	}

	var err error
	var portState PDState

	//
	// Fill in fields common to all versions (TCPM and PDC)
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

	if t[pdStateFieldIndex[ver]["VConn"]] == "-VC" {
		portState.VConn = true
	} else {
		portState.VConn = false
	}

	connection, err := t.lookup("Connection", ver)
	if err != nil {
		return nil, err
	}
	portState.Connection = connectionValue(connection)

	//
	// PE state and flags (TCPMv1 and TCPMv2 only)
	//
	if ver == TCPMv2 || ver == TCPMv1 {
		// PE State name
		portState.PEStateName, err = t.peStateName(ver)
		if err != nil {
			return nil, err
		}

		// PE Flags
		flags64, err := strconv.ParseUint(t[pdStateFieldIndex[ver]["PEFlags"]], 16, 32)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert PE flags number")
		}
		portState.PEFlags = uint32(flags64)
	}

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

	//
	// PDC State (PDC only)
	//

	if ver == PDC {
		portState.PDCState = t[pdStateFieldIndex[ver]["PDCState"]]
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
