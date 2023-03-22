// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/util"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type chargeState struct {
	command        string
	state          string
	expectedOutput string
}

var (
	coDontCharge               = chargeState{"chargeoverride", "dontcharge", "Override port set to -2"}
	coOff                      = chargeState{"chargeoverride", "off", "Override port set to -1"}
	ccDischarge                = chargeState{"chargecontrol", "discharge", "Charge state machine force discharge."}
	ccNormal                   = chargeState{"chargecontrol", "normal", "Charge state machine is in normal mode."}
	boardsCannotChargeOverride = []string{"jacuzzi", "jacuzzi64"}
)

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func supportChargeOverride() bool {
	board := util.GetBoard()
	return !contains(boardsCannotChargeOverride, board)
}

func setChargeState(ctx context.Context, s chargeState) error {
	stdout, stderr, err := testexec.CommandContext(ctx, "ectool", s.command, s.state).SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "unable to set battery charge to %s, got error %s", s.state, string(stderr))
	}
	if output := strings.TrimSpace(string(stdout)); output != s.expectedOutput {
		return errors.Errorf("unexpected output: got %s; want %s", output, s.expectedOutput)
	}
	return nil
}

// SetBatteryDischarge forces the battery to discharge. This will fail if the
// remaining battery charge is lower than lowBatteryCutoff.
func SetBatteryDischarge(ctx context.Context, expectedMaxCapacityDischarge float64) (CleanupCallback, error) {
	shutdownCutoff, err := power.LowBatteryShutdownPercent(ctx)
	if err != nil {
		return nil, err
	}
	lowBatteryCutoff := shutdownCutoff + expectedMaxCapacityDischarge
	devPath, err := power.SysfsBatteryPath(ctx)
	if err != nil {
		return nil, err
	}
	capacity, err := power.ReadBatteryCapacity(ctx, devPath)
	if err != nil {
		return nil, err
	}
	energy, err := power.ReadBatteryEnergy(ctx, devPath)
	if err != nil {
		return nil, err
	}
	status, err := power.ReadBatteryStatus(ctx, devPath)
	if err != nil {
		return nil, err
	}
	if status == power.BatteryStatusDischarging {
		testing.ContextLog(ctx, "WARNING Battery is already discharging")
	}

	testing.ContextLog(ctx, "Setting battery to discharge. Current capacity: ", capacity, "% (", energy, "Wh), Shutdown cutoff: ", shutdownCutoff, "%, Expected maximum discharge during test: ", expectedMaxCapacityDischarge, "%")
	if lowBatteryCutoff >= capacity {
		return nil, errors.Errorf("battery percent %.2f is too low to start discharging", capacity)
	}

	useChargeOverride := supportChargeOverride()
	chargeState := coDontCharge
	if !useChargeOverride {
		chargeState = ccDischarge
	}
	if err := setChargeState(ctx, chargeState); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		dischargePercent := 0.0
		dischargeWh := 0.0
		capacityAfterTest, err := power.ReadBatteryCapacity(ctx, devPath)
		if err == nil {
			dischargePercent = capacity - capacityAfterTest
		}
		energyAfterTest, err := power.ReadBatteryEnergy(ctx, devPath)
		if err == nil {
			dischargeWh = energy - energyAfterTest
		}
		// We reset the battery discharge mode to normal (charging), even if it
		// wasn't set before the test because leaving the device disharging
		// could cause a device to shut down.
		testing.ContextLog(ctx, "Resetting battery discharge to normal. Discharge during test: ", dischargePercent, "% (", dischargeWh, "Wh)")
		chargeState := coOff
		if !useChargeOverride {
			chargeState = ccNormal
		}
		return setChargeState(ctx, chargeState)
	}, nil
}

// AllowBatteryCharging will re-enable AC power and allow the battery to charge.
func AllowBatteryCharging(ctx context.Context) error {
	chargeState := coOff
	if !supportChargeOverride() {
		chargeState = ccNormal
	}
	return setChargeState(ctx, chargeState)
}
