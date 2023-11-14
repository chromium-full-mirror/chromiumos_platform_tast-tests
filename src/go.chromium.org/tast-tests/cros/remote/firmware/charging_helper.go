// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements functions to interact with servo to perform
// power related operations.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// GetChargingState returns map[string]string of parsed chgstate output
// from EC, in ideal situation this would be just predefined struct with
// fields for each value, but the EC console output seems to be varying
// between platforms and firmware versions in terms of fields' order,
// count and categories. This approach might look less elegant, but ii's
// safer and provides a nice interface to extract data
func GetChargingState(ctx context.Context, h *Helper) (map[string]string, error) {
	chgstateOutput, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "chgstate", []string{`.*\ndebug output = .+\n`})
	if err != nil {
		return nil, errors.Wrap(err, "EC chgstate command failed")
	}

	var (
		category string
		key      string
		value    string
	)

	cstateMap := make(map[string]string)

	// For reference, the current output of "chgstate" EC command is provided below
	// in shortened form, actual field names and values might be different per board
	// If you notice any issues with parsing on newer EC firmware, change the parsing
	// method accordingly.
	// Example output of "chgstate":
	//   state = charge
	//   ac = 1
	//   batt_is_charging = 1
	//   chg.*:
	//     voltage = 8648mV
	//     current = 0mA
	//     (...)
	//   batt.*:
	//     temperature = 24C
	//     state_of_charge = 100%
	//     voltage = 8543mV
	//     current = 0mA
	//     (...)
	//   requested_voltage = 0mV
	//   requested_current = 0mA
	//   chg_ctl_mode = 0
	//   (...)
	for _, line := range strings.Split(chgstateOutput[0][0], "\n") {

		if strings.Contains(line, "*") {
			category = strings.Split(line, ".")[0]
		}
		if strings.Contains(line, "=") {
			if !strings.HasPrefix(line, "\t") {
				category = "global"
			}

			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSpace(line)
			key = strings.Split(line, " = ")[0]
			value = strings.Split(line, " = ")[1]

			cstateMap[category+"."+key] = value
		}
	}

	return cstateMap, nil
}

// ChargeToLevel waits for the DUT to charge to minimum battery level
func ChargeToLevel(ctx context.Context, h *Helper, minBatteryLevel int, timeout time.Duration) error {
	testing.ContextLogf(ctx, "Wait for DUT battery charge to exceed %d%%", minBatteryLevel)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		chargeState, err := GetChargingState(ctx, h)
		if err != nil {
			return testing.PollBreak(err)
		}

		batteryLevel, err := strconv.Atoi(strings.Trim(chargeState["batt.state_of_charge"], "%"))
		if err != nil {
			return testing.PollBreak(err)
		}

		testing.ContextLogf(ctx, "Current battery state of charge %d%%", batteryLevel)

		if batteryLevel >= minBatteryLevel {
			// Done charging
			return nil
		}

		if chargeState["global.batt_is_charging"] != "1" {
			return testing.PollBreak(errors.Errorf("Battery not charging, current level %d%%", batteryLevel))
		}

		return errors.Errorf("battery level %d%% too low", batteryLevel)
	}, &testing.PollOptions{
		Timeout:  timeout,
		Interval: 30 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to charge battery")
	}

	return nil
}

const (
	// EC Battery states:

	// ECFullyDischarged is the status code for Fully Discharged battery state.
	ECFullyDischarged = 0x10 // (1 << 4)
	// ECFullyCharged is the status code for Fully Charged battery state.
	ECFullyCharged = 0x20 // (1 << 5)
	// ECDischarging is the status code for Discharging battery state.
	ECDischarging = 0x40 // (1 << 6)
	// ECInitializing is the status code for Initializing battery state.
	ECInitializing = 0x80 // (1 << 7)

	// EC Battery alarms:

	// ECRemainingTimeAlarm is the status code for Remaining Time battery alarm.
	ECRemainingTimeAlarm = 0x100 // (1 << 8)
	// ECRemainingCapacityAlarm is the status code for Remaining Capacity battery alarm.
	ECRemainingCapacityAlarm = 0x200 // (1 << 9)
	// ECTerminateDischargeAlarm is the status code for Terminate Discharge battery alarm.
	ECTerminateDischargeAlarm = 0x800 // (1 << 11)
	// ECOverTempAlarm is the status code for Over Temp battery alarm.
	ECOverTempAlarm = 0x1000 // (1 << 12)
	// ECTerminateChargeAlarm is the status code for Terminate Charge battery alarm.
	ECTerminateChargeAlarm = 0x4000 // (1 << 14)
	// ECOverChargedAlarm is the status code for Over Charged battery alarm.
	ECOverChargedAlarm = 0x8000 // (1 << 15)
)

// ECBatteryState is a struct containing information about the current battery state
// as reported by the ec console.
type ECBatteryState struct {
	StatusCode int64
	Status     []string
	Alarms     []string
	Params     int64
	Charging   string
	Charge     float64
	Display    float64
}

// BatteryStatusCodeToName is a map of battery status to human readable name.
var BatteryStatusCodeToName = map[int]string{
	ECFullyDischarged:         "Fully Discharged",
	ECFullyCharged:            "Fully Charged",
	ECDischarging:             "Discharging",
	ECInitializing:            "Initialized",
	ECRemainingTimeAlarm:      "Remaining Time Alarm",
	ECRemainingCapacityAlarm:  "Remaining Capacity Alarm",
	ECTerminateDischargeAlarm: "Terminate Discharge Alarm",
	ECOverTempAlarm:           "Over Temp Alarm",
	ECTerminateChargeAlarm:    "Terminate Charge Alarm",
	ECOverChargedAlarm:        "Over Charged Alarm",
}

// BatteryStateToStatusCode is a map of battery states and alarms to a status code.
var BatteryStateToStatusCode = map[string]int{
	"EMPTY": ECFullyDischarged,
	"FULL":  ECFullyCharged,
	"DCHG":  ECDischarging,
	"INIT":  ECInitializing,
	"RT":    ECRemainingTimeAlarm,
	"RC":    ECRemainingCapacityAlarm,
	"TD":    ECTerminateDischargeAlarm,
	"OT":    ECOverTempAlarm,
	"TC":    ECTerminateChargeAlarm,
	"OC":    ECOverChargedAlarm,
}

// GetECBatteryStatus returns a struct containing information about the battery pulled from the ec console.
func GetECBatteryStatus(ctx context.Context, h *Helper) (*ECBatteryState, error) {
	// Example ec battery output:
	// Status:    0x00e7 FULL DCHG INIT
	// Param flags:00000002
	// Charging:  Not Allowed
	// Charge:    100 %
	//   Display:  100 %               (...)

	reStatus := `Status:\s*0[xX]([0-9a-fA-F]+)\s+((?:(?:EMPTY|FULL|DCHG|INIT)\s+)*)((?:(?:RT|RC|\--|TD|OT|TC|OC)\s+)*)[\r\n]`
	reParam := `Param\s*flags:\s*([0-9a-fA-F]+)`
	reCharging := `Charging:\s*(Allowed|Not Allowed)`
	reCharge := `Charge:\s*(\d+)\s*\%`
	reDisplay := `Display:\s*(\d+\.\d+)\s*\%`

	batteryPattern := []string{reStatus, reParam, reCharging, reCharge, reDisplay}

	var statusCode, params int64
	var charge, display float64
	status := make([]string, 0)
	alarms := make([]string, 0)
	var charging string

	// Get battery info from EC, retry in case output is corrupted/interrupted.
	out, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "battery", batteryPattern)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run 'battery' command in ec console")
	}

	statusCode, err = strconv.ParseInt(out[0][1], 16, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse battery status as int")
	}
	statusRaw := strings.Split(strings.TrimSpace(out[0][2]), " ")
	for _, v := range statusRaw {
		if v == "" {
			continue
		}
		sc, ok := BatteryStateToStatusCode[v]
		if !ok {
			return nil, errors.Errorf("unrecognized status %v in battery state", v)
		}
		s, ok := BatteryStatusCodeToName[sc]
		if !ok {
			return nil, errors.Errorf("unrecognized charge status code %v in battery state", sc)
		}
		status = append(status, s)
	}
	alarmsRaw := strings.Split(strings.TrimSpace(out[0][3]), " ")
	for _, v := range alarmsRaw {
		if v == "" {
			continue
		}
		sc, ok := BatteryStateToStatusCode[v]
		if !ok {
			return nil, errors.Errorf("unrecognized alarm %v in battery state", v)
		}
		s, ok := BatteryStatusCodeToName[sc]
		if !ok {
			return nil, errors.Errorf("unrecognized alarm status code %v in battery state", sc)
		}
		alarms = append(alarms, s)
	}
	params, err = strconv.ParseInt(out[1][1], 16, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse battery params as int")
	}
	charging = out[2][1]
	charge, err = strconv.ParseFloat(out[3][1], 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse battery charge as float")
	}
	display, err = strconv.ParseFloat(out[4][1], 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse displayed charge as float")
	}

	state := &ECBatteryState{
		StatusCode: statusCode,
		Status:     status,
		Alarms:     alarms,
		Params:     params,
		Charging:   charging,
		Charge:     charge,
		Display:    display,
	}

	return state, nil
}

// PollToSetChargerStatus polls to connect/disconnect ac charger and confirms state change from ec.
func PollToSetChargerStatus(ctx context.Context, h *Helper, attachCharger bool) error {
	srcOrSnk := "source"
	if !attachCharger {
		srcOrSnk = "sink"
	}

	// Verify that DUT charger is in expected state.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Set servo role to %s", srcOrSnk)
		if err := h.SetDUTPower(ctx, attachCharger); err != nil {
			return errors.Wrapf(err, "failed to set servo role to %s", srcOrSnk)
		}

		ok, err := h.Servo.GetChargerAttached(ctx)
		if err != nil {
			testing.ContextLog(ctx, "GetChargerAttached failed: ", err)
			return errors.Wrap(err, "error checking whether charger is attached")
		} else if ok != attachCharger {
			testing.ContextLogf(ctx, "GetChargerAttached got %v, want %v", ok, attachCharger)
			return errors.Errorf("expected charger attached state: %v", attachCharger)
		}
		return nil
		// Metaknight takes a worst case of ~120s to notice the charger, so retry for 200s instead.
	}, &testing.PollOptions{Timeout: 200 * time.Second, Interval: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to check if charger is attached")
	}
	return nil
}

func unitValue(raw, unit string) (int, error) {
	valueStr := strings.TrimSuffix(raw, unit)
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse value %v to int", valueStr)
	}
	return value, nil
}

// CheckChargingState verifies voltage values are at expected values.
func CheckChargingState(ctx context.Context, h *Helper) error {
	cs, err := GetChargingState(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed querying EC")
	}

	if cs["global.ac"] != "1" {
		return errors.New("the DUT is not plugged to AC charger")
	}

	if cs["global.state"] != "charge" {
		return errors.New("the DUT is not charging (DUT is on AC but does not report charging)")
	}

	if val, err := unitValue(cs["batt.current"], "mA"); err != nil {
		return errors.Wrap(err, "failed to parse batt.current")
	} else if val <= 0 {
		return errors.Errorf("DUT is not charging (battery current below/equal to zero: %s)", cs["batt.current"])
	}
	if val, err := unitValue(cs["chg.voltage"], "mV"); err != nil {
		return errors.Wrap(err, "failed to parse chg.voltage")
	} else if val <= 0 {
		return errors.Errorf("DUT is not charging (charger voltage below/equal to zero: %s)", cs["chg.voltage"])
	}

	desiredCurrent, err := unitValue(cs["batt.desired_current"], "mA")
	if err != nil {
		return errors.Wrap(err, "failed to parse batt.desired_current")
	}

	stateOfCharge, err := unitValue(cs["batt.state_of_charge"], "%")
	if err != nil {
		return errors.Wrap(err, "failed to parse batt.state_of_charge")
	}
	if (desiredCurrent < 100) && (stateOfCharge < 100) {
		return errors.Errorf("Trickle charging battery? Need more discharge? (desired current: %s)",
			cs["batt.desired_current"])
	}

	// Check the requested vs actual voltage.
	current, err := unitValue(cs["chg.voltage"], "mV")
	if err != nil {
		return errors.Wrap(err, "failed to parse chg.voltage")
	}
	desired, err := unitValue(cs["batt.desired_voltage"], "mV")
	if err != nil {
		return errors.Wrap(err, "failed to parse batt.desired_voltage")
	}
	if float32(current) > 1.05*float32(desired) {
		return errors.Errorf("Charger target voltage is too high. (target: %s, battery: %s)",
			cs["chg.voltage"], cs["batt.desired_voltage"])
	}

	current, err = unitValue(cs["chg.voltage"], "mV")
	if err != nil {
		return errors.Wrap(err, "failed to parse chg.voltage")
	}
	desired, err = unitValue(cs["batt.voltage"], "mV")
	if err != nil {
		return errors.Wrap(err, "failed to parse batt.desired_voltage")
	}
	if float32(desired) > 1.05*float32(current) {
		return errors.Errorf("Battery actual voltage is too high. (battery: %s, charger: %s)",
			cs["batt.voltage"], cs["chg.voltage"])
	}

	// Check the requested vs actual Current.
	current, err = unitValue(cs["chg.current"], "mA")
	if err != nil {
		return errors.Wrap(err, "failed to parse chg.current")
	}
	desired, err = unitValue(cs["batt.desired_current"], "mA")
	if err != nil {
		return errors.Wrap(err, "failed to parse batt.desired_current")
	}
	if float32(current) > 1.05*float32(desired) {
		return errors.Errorf("Charger target current is too high. (target: %s, battery: %s)",
			cs["chg.current"], cs["batt.desired_current"])
	}

	current, err = unitValue(cs["chg.current"], "mA")
	if err != nil {
		return errors.Wrap(err, "failed to parse chg.current")
	}
	desired, err = unitValue(cs["batt.current"], "mA")
	if err != nil {
		return errors.Wrap(err, "failed to parse batt.current")
	}
	if float32(desired) > 1.05*float32(current) {
		return errors.Errorf("Battery actual current is too high. (battery: %s, charger: %s)",
			cs["batt.current"], cs["chg.current"])
	}

	return nil
}

const (
	// KernelFullyCharged indicates kernel reports battery is fully charged.
	KernelFullyCharged string = "fully charged"
	// KernelCharging indicates kernel reports battery is charging.
	KernelCharging string = "charging"
	// KernelNotCharging indicates kernel reports battery is not charging.
	KernelNotCharging string = "not charging"
	// KernelDischarging indicates kernel reports battery is discharging.
	KernelDischarging string = "discharging"
)

// GetKernelBatteryState returns battery state as reported by kernel.
func GetKernelBatteryState(ctx context.Context, h *Helper) (string, error) {
	testing.ContextLog(ctx, "Get battery status reported by kernel")
	out, err := h.DUT.Conn().CommandContext(ctx, "power_supply_info").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get power supply info")
	}

	// Matches the state property under the device battery.
	reBattState := regexp.MustCompile(`(?s)Device: Battery.*state:(?-s)\s+(\S+(\s\S+)*)\s`)
	match := reBattState.FindSubmatch(out)
	if match == nil {
		return "", errors.Errorf("failed to parse battery state in output: %s", string(out))
	}
	testing.ContextLog(ctx, "Kernel reported battery status: ", string(match[1]))

	batteryState := strings.ToLower(strings.TrimSpace(string(match[1])))
	switch batteryState {
	case KernelFullyCharged, KernelCharging, KernelNotCharging, KernelDischarging:
		return batteryState, nil
	default:
		return "", errors.Errorf("got unexpected battery state from kernel %v", batteryState)
	}
}
