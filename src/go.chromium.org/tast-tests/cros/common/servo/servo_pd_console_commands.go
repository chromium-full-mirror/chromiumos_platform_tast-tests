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

type servoStateTokens []string

func (t *servoStateTokens) lookup(field string) (string, error) {
	token := (*t)[pdFieldIndex[field]]
	if value, ok := pdFieldLookup[field][token]; ok {
		return value, nil
	}
	return "", errors.Errorf("PD field %q contains unknown value %q", field, token)
}

// A PDState encapsulates the full PD port state on a servo.
type PDState struct {
	Port        int
	Polarity    pdPolarityValue
	Connection  connectionValue
	PowerRole   powerRoleValue
	DataRole    dataRoleValue
	PEState     int
	PEStateName string
	Flags       uint32
}

const (
	// reServoPdStateCommand is a regex to retrieve the PD state, valid for TCPM v1 only.
	// Example: Port C1 CC1, Ena - Role: SRC-UFP State: 23(), Flags: 0x1415e
	reServoPdStateCommand string = `Port\s+C(\d+)\s+(CC\d+),\s+(\S+)\s+-\s+Role:\s+(\w+)-(\w+)\s+State:\s(\d+)\(.*\),\s+Flags:\s+0x(\w*)[\r\n]`
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

// pdFieldLookup maps pdFieldIndex key values to the valid servo output that is returned
// from the "pd state" command.
var pdFieldLookup = map[string]map[string]string{
	"CCPolarity": {"CC1": string(PolarityCC1), "CC2": string(PolarityCC2),
		"CC3": string(PolarityCC1DTS), "CC4": string(PolarityCC2DTS)},
	"Connection": {"Ena": string(PDEnabled), "Dis": string(PDDisabled)},
	"PowerRole":  {"SRC": string(PowerRoleSRC), "SNK": string(PowerRoleSNK)},
	"DataRole":   {"DFP": string(DataRoleDFP), "UFP": string(DataRoleUFP)},
}

// Maps PEState ints to a friendly string name based on EC's `include/usb_pd.h`
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

// getServoPDStateHelper queries a specific Servo port's PD state. Use GetServoPDState()
// for DUT port and GetServoChargerPortPDState() for charger port instead.
func (s *Servo) getServoPDStateHelper(ctx context.Context, port int) (*PDState, error) {
	// For servoV4 and servoV4p1, the PD port 0 is the charging port and PD port 1 is
	// the DUT port.
	var portState PDState
	out, err := s.RunServoCommandGetOutput(ctx, fmt.Sprintf("pd %d state", port), []string{reServoPdStateCommand})
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
	portState.Polarity = pdPolarityValue(polarity)

	connection, err := t.lookup("Connection")
	if err != nil {
		return nil, err
	}
	portState.Connection = connectionValue(connection)

	powerRole, err := t.lookup("PowerRole")
	if err != nil {
		return nil, err
	}
	portState.PowerRole = powerRoleValue(powerRole)

	dataRole, err := t.lookup("DataRole")
	if err != nil {
		return nil, err
	}
	portState.DataRole = dataRoleValue(dataRole)

	portState.PEState, err = strconv.Atoi(t[pdFieldIndex["PEState"]])
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert PE state number")
	}

	if peStateName, ok := peStateNameLookup[portState.PEState]; ok {
		portState.PEStateName = peStateName
	} else {
		portState.PEStateName = fmt.Sprintf("Unknown (%d)", portState.PEState)
	}

	flags64, err := strconv.ParseUint(t[pdFieldIndex["Flags"]], 16, 32)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert flags number")
	}
	portState.Flags = uint32(flags64)

	return &portState, nil
}

// ServoSendDataSwapRequest initiates a data swap request from the servo's PD port.
func (s *Servo) ServoSendDataSwapRequest(ctx context.Context) (pdControlMsgType, error) {
	// Enable PD message so we can check the response from the DUT.
	err := s.RunServoCommand(ctx, "pd dump 2")
	if err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to send enable PD debug")
	}
	// Always disable PD commands on exit.
	defer s.RunServoCommand(ctx, "pd dump 0")

	out, err := s.RunServoCommandGetOutput(ctx, "pd 1 swap data", []string{reEcPdRecv})
	if err != nil {
		return PDCtrlReserved, errors.Wrap(err, "failed to send servo data swap")
	}

	recvMsg, err := strconv.ParseUint(out[0][1], 16, 32)
	if err != nil {
		return PDCtrlReserved, errors.Wrapf(err, "failed to convert swap RECV message %q", out[0][1])
	}

	replyValue := int(recvMsg & PdControlMsgMask)
	if reply, ok := pdControlMsg[replyValue]; ok {
		return reply, nil
	}

	return PDCtrlReserved, errors.Errorf("unknown PD control message value %q", replyValue)
}

// GetServoPDState returns the state of the PD port on the servo that connects to the DUT
func (s *Servo) GetServoPDState(ctx context.Context) (*PDState, error) {
	return s.getServoPDStateHelper(ctx, 1)
}

// GetServoChargerPortPDState returns the state of the PD port on the servo that connects
// to the charger (C0)
func (s *Servo) GetServoChargerPortPDState(ctx context.Context) (*PDState, error) {
	return s.getServoPDStateHelper(ctx, 0)
}

// RequireChargerAttached verifies that the Servo charger port (#0) is an active sink
func (s *Servo) RequireChargerAttached(ctx context.Context) error {

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := s.GetServoChargerPortPDState(ctx)
		if err != nil {
			return testing.PollBreak(
				errors.Wrap(err, "cannot access charger port PD status"),
			)
		}

		testing.ContextLogf(ctx, "C0 PE State is %s", pdState.PEStateName)

		if pdState.PEStateName != "PD_STATE_SNK_READY" {
			return errors.New("Servo charger port (C0) is not PD_STATE_SNK_READY")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return err
	}

	return nil
}
