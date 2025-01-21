// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package battery provides control for a DUT's battery.
package battery

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Battery representation with some methods, reduce number of args passed to functions.
type Battery struct {
	h               *firmware.Helper
	voltageCharging int
	outdir          string
}

// GetBatteryProxy creates and returns Battery type.
func GetBatteryProxy(ctx context.Context, h *firmware.Helper) *Battery {
	return &Battery{
		h:               h,
		voltageCharging: 0,
		outdir:          "",
	}
}

const (
	// charging
	chargingPollTimeout          = 240 * time.Minute
	chargingPollInterval         = 100 * time.Millisecond
	minimumBatteryLevelToPowerUp = 5
	// discharging
	dischargingPollInterval = 1 * time.Minute
)

// SetChargingVoltageAndCharge enables charging with provided voltage.
func (batt *Battery) SetChargingVoltageAndCharge(ctx context.Context, voltage int) error {
	batt.voltageCharging = voltage
	testing.ContextLogf(ctx, "Battery charge using %dV", voltage)

	if err := firmware.PollToSetChargerStatus(ctx, batt.h, true); err != nil {
		return errors.Wrap(err, "failed to connect charger")
	}
	testing.ContextLogf(ctx, "Setting usbc chg to %dV", voltage)
	command := fmt.Sprintf("usbc chg %d", voltage)
	if err := batt.h.Servo.RunServoCommand(ctx, command); err != nil {
		return errors.Wrap(err, "failed to set voltage level to charge")
	}

	return nil
}

// SetOutdir sets outdir for saving metrics.
func (batt *Battery) SetOutdir(path string) {
	batt.outdir = path
}

// ChargeAndMonitorUpTo maintains charging state, save and validate provided power.
func (batt *Battery) ChargeAndMonitorUpTo(ctx context.Context, limit int, validateCharging func(DataPoint)) error {

	batteryMonitor := NewMetricMonitor()
	testing.ContextLogf(ctx, "Wait for DUT battery charge to exceed %d%%", limit)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var chargeState map[string]string
		var batteryLevel int
		var err error
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			chargeState, batteryLevel, err = getChargingStateAndBatteryLevel(ctx, batt.h)
			if err != nil {
				return errors.Wrap(err, "failed to charge battery")
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
			return testing.PollBreak(err)
		}

		if batteryLevel >= limit {
			// Done charging.
			return nil
		}

		var snapshot DataPoint
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			snapshot, err = readChargingAndBatteryParams(ctx, batt.h, chargeState)
			if err != nil {
				return testing.PollBreak(err)
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
			return testing.PollBreak(err)
		}

		batteryMonitor.AddMetric(snapshot)
		validateCharging(snapshot)
		apOn, err := isAPOn(ctx, batt.h)
		if err != nil {
			testing.ContextLog(ctx, "Couldn't get AP state")
		}
		if batteryLevel >= minimumBatteryLevelToPowerUp && !apOn {
			if err := turnOnDUT(ctx, batt.h); err != nil {
				return testing.PollBreak(err)
			}
		}
		return errors.Errorf("battery level %d%% too low", batteryLevel)
	}, &testing.PollOptions{
		Timeout:  chargingPollTimeout,
		Interval: chargingPollInterval,
	}); err != nil {
		return errors.Wrap(err, "failed to charge battery")
	}
	if err := saveBatteryMetrics(ctx, batt.outdir, batteryMonitor, batt.voltageCharging); err != nil {
		return errors.Wrap(err, "failed to save battery metrics")
	}
	return nil
}

// DischargeBatteryWithTimeout uses stressing CPU to drain battery with timeout to the target level, provided as an argument.
func (batt *Battery) DischargeBatteryWithTimeout(ctx context.Context, percentBattDischargeLevel float64, minutesTimeout time.Duration) error {
	testing.ContextLog(ctx, "Disconnecting charger")
	if err := firmware.PollToSetChargerStatus(ctx, batt.h, false); err != nil {
		return errors.Wrap(err, "failed to disconnect charger")
	}
	testing.ContextLog(ctx, "Initiating battery discharging")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		battery, err := getBatteryLevel(ctx, batt.h)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery or ap state: ", err)
			return testing.PollBreak(err)
		}
		testing.ContextLogf(ctx, "Current charge: %v, target: %v", battery.Charge, percentBattDischargeLevel)
		apOn, err := isAPOn(ctx, batt.h)
		if err != nil {
			testing.ContextLog(ctx, "Couldn't get AP state")
		}
		if battery.Charge > float64(percentBattDischargeLevel) && apOn {
			testing.ContextLog(ctx, "Stressing CPU to discharge battery")
			if err := stressCPU(ctx, batt.h); err != nil {
				return errors.Wrap(err, "failed to stress CPU")
			}
			// More power consuming action can be added here.
			return errors.Errorf("Not enough battery discharged: %v, want %v", battery.Charge, percentBattDischargeLevel)
		}

		return nil
	}, &testing.PollOptions{Timeout: minutesTimeout, Interval: dischargingPollInterval}); err != nil {
		return errors.Wrap(err, "failed to discharge battery")
	}
	return nil
}

func getBatteryLevel(ctx context.Context, h *firmware.Helper) (*firmware.ECBatteryState, error) {
	var battery *(firmware.ECBatteryState)
	var err error

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		battery, err = firmware.GetECBatteryStatus(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to get current battery status")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to poll for battery status from EC")
	}

	return battery, nil
}

func isAPOn(ctx context.Context, h *firmware.Helper) (bool, error) {
	var apPower string
	var err error

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		apPower, err = h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get AP state")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return false, errors.Wrap(err, "failed to poll for battery status from EC")
	}

	return strings.Compare(apPower, "S0") == 0, nil
}

func saveBatteryMetrics(ctx context.Context, outdir string, batteryMonitor *MetricMonitor, voltage int) error {
	fileName := fmt.Sprintf("chart_%dV_charging.csv", voltage)
	filePath := filepath.Join(outdir, fileName)
	testing.ContextLogf(ctx, "Saving metrics to %s", filePath)

	if err := batteryMonitor.SaveCSV(filePath); err != nil {
		return errors.Wrapf(err, "failed to save metrics to %s/%s", filePath, fileName)
	}
	return nil
}

func getChargingStateAndBatteryLevel(ctx context.Context, h *firmware.Helper) (map[string]string, int, error) {
	chargeState, err := firmware.GetChargingState(ctx, h)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get charging state")
	}

	batteryLevel, err := strconv.Atoi(strings.Trim(chargeState["batt.state_of_charge"], "%"))
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to parse battery level")
	}
	return chargeState, batteryLevel, nil
}

func readChargingAndBatteryParams(ctx context.Context, h *firmware.Helper, chargeState map[string]string) (DataPoint, error) {
	batteryLevel, err := strconv.Atoi(strings.Trim(chargeState["batt.state_of_charge"], "%"))
	if err != nil {
		return DataPoint{}, errors.Wrap(err, "failed to parse batt.state_of_charge")
	}

	currentServoVoltageValue, err := h.Servo.GetFloat(ctx, "servo_v4p1.vbus_voltage")
	if err != nil {
		return DataPoint{}, errors.Wrap(err, "failed to parse servo_v4p1.vbus_voltage")
	}

	currentServoCurrentValue, err := h.Servo.GetFloat(ctx, "servo_v4p1.vbus_current")
	if err != nil {
		return DataPoint{}, errors.Wrap(err, "failed to parse servo_v4p1.vbus_current")
	}

	return DataPoint{
		Timestamp:            time.Now().Format("15:04:05.000"),
		BatteryPercent:       float64(batteryLevel),
		ServoChargingVoltage: float64(currentServoVoltageValue),
		ServoChargingCurrent: float64(currentServoCurrentValue),
	}, nil

}

// ValidateCharging callback function that validates measurements in the test.
func ValidateCharging(ctx context.Context, data DataPoint, desired, deviationPercent int) {
	servoVoltageValue := data.ServoChargingVoltage / 1000.0
	topThreshold := float64(desired) * (1.0 + float64(deviationPercent)/100.0)
	bottomThreshold := float64(desired) * (1.0 - float64(deviationPercent)/100.0)

	if servoVoltageValue > topThreshold || servoVoltageValue < bottomThreshold {
		testing.ContextLogf(ctx, "[WARNING] voltage is outside of expected range, got %.3fV, expected %dV with %d%% deviation (%.3f, %.3f)",
			servoVoltageValue, desired, deviationPercent, bottomThreshold, topThreshold)
	}
}

func turnOnDUT(ctx context.Context, h *firmware.Helper) error {
	apOn, err := isAPOn(ctx, h)
	if err != nil {
		return errors.Wrap(err, "couldn't get AP state")
	}
	if !apOn {
		testing.ContextLog(ctx, "Pressing power key to turn on DUT")
		if err := firmware.BootDutViaPowerPress(ctx, h, h.DUT); err != nil {
			return errors.Wrap(err, "failed to turn on DUT")
		}
	}
	return nil
}

func stressCPU(ctx context.Context, h *firmware.Helper) error {
	script := "cd /usr/local/bin; stress-ng --cpu 64 --timeout 1m"
	cmd := h.DUT.Conn().CommandContext(ctx, "sh", "-c", script)
	if !h.DUT.Connected(ctx) {
		h.DUT.Connect(ctx)
	}
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to stress CPU")
	}
	return nil
}
