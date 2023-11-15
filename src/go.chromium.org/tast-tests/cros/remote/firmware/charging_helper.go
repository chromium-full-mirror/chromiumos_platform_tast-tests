// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements functions to interact with servo to perform
// power related operations.

package firmware

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
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
