// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/power"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const batteryCheckInterval = time.Second

// BatteryInfoTracker is a helper to collect battery info.
type BatteryInfoTracker struct {
	prefix                    string
	batteryPath               string
	chargeFullDesign          float64
	chargeNow                 []float64 // Remaining charge [Ah].
	voltageMinDesign          float64
	voltageMaxDesign          float64
	lowBatteryShutdownPercent float64
	lowBatteryShutdownTime    int64
	batteryChargeStart        float64
	batteryChargeEnd          float64
	batteryCapacityStart      float64
	batteryCapacityEnd        float64
	energy                    float64   // Total energy consumed.
	power                     []float64 // Power reading every |batteryCheckInterval|.
	powerTime                 []float64
	energyFullDesign          float64
	cycleCount                int64
	collecting                chan bool
	collectingErr             chan error
	err                       error
	duration                  time.Duration
	trackStartTime            time.Time
}

// NewBatteryInfoTracker creates a new instance of BatteryInfoTracker. If battery is not
// used on the device, available flag is set to false and makes track a no-op.
func NewBatteryInfoTracker(ctx context.Context, metricPrefix string) (*BatteryInfoTracker, error) {
	batteryPath, err := power.SysfsBatteryPath(ctx)
	if err != nil {
		// Some devices (e.g. chromeboxes) do not have the battery, but that's fine
		// for now.
		// TODO(b/180915240): find the way to measure power data on those devices.
		testing.ContextLog(ctx, "Failed to get battery path: ", err)
		testing.ContextLog(ctx, "This might be okay. Continue the test without battery info")
		return nil, nil
	}

	// Drallion does not support automated power testing (see b/277659498#comment4).
	model, err := crosconfig.Get(ctx, "/", "name")
	if err != nil {
		return nil, errors.Wrap(err, "could not find model name")
	}
	if strings.Contains(model, "drallion") {
		testing.ContextLog(ctx, "Drallion does not support automated power testing, continuing the test without battery info")
		return nil, nil
	}

	chargeFullDesign, err := power.ReadBatteryProperty(ctx, batteryPath, "charge_full_design")
	if err != nil {
		return nil, err
	}
	voltageMinDesign, err := power.ReadBatteryProperty(ctx, batteryPath, "voltage_min_design")
	if err != nil {
		return nil, err
	}
	voltageMaxDesign, err := power.ReadBatteryProperty(ctx, batteryPath, "voltage_max_design")
	if err != nil {
		// Change to raise error if voltageMaxDesign is later used.
		testing.ContextLog(ctx, "Battery does not have field: voltage_max_design")
	}
	cycleCount, err := power.ReadBatteryIntProperty(ctx, batteryPath, "cycle_count")
	if err != nil {
		// Change to raise error if cycleCount is later used.
		testing.ContextLog(ctx, "Battery does not have field: cycle_count")
	}
	output, err := testexec.CommandContext(ctx, "check_powerd_config", "--low_battery_shutdown_percent").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read low battery shutdown percent from powerd")
	}
	lowBatteryShutdownPercent, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q", output)
	}
	powerdRead, err := testexec.CommandContext(ctx, "check_powerd_config", "--low_battery_shutdown_time").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read low battery shutdown time from powerd")
	}
	lowBatteryShutdownTime, err := strconv.ParseInt(strings.TrimSpace(string(powerdRead)), 0, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q", powerdRead)
	}

	return &BatteryInfoTracker{
		prefix:                    metricPrefix,
		batteryPath:               batteryPath,
		chargeFullDesign:          chargeFullDesign,
		cycleCount:                cycleCount,
		voltageMinDesign:          voltageMinDesign,
		voltageMaxDesign:          voltageMaxDesign,
		lowBatteryShutdownPercent: lowBatteryShutdownPercent,
		lowBatteryShutdownTime:    lowBatteryShutdownTime,
	}, nil
}

// Start indicates that the battery tracking should start. It sets the batteryChargeStart value.
func (t *BatteryInfoTracker) Start(ctx context.Context, timeZero time.Time) error {
	if t == nil {
		return nil
	}

	if t.collecting != nil {
		return errors.New("already started")
	}
	t.collecting = make(chan bool)
	t.collectingErr = make(chan error, 1)

	if !t.trackStartTime.IsZero() {
		return errors.New("Battery info tracker already started")
	}

	chargeNow, err := power.ReadBatteryProperty(ctx, t.batteryPath, "charge_now")
	if err != nil {
		return err
	}
	capacityNow, err := power.ReadBatteryCapacity(ctx, t.batteryPath)
	if err != nil {
		return err
	}

	t.trackStartTime = time.Now() // Reset start time.
	t.batteryChargeStart = chargeNow
	t.batteryCapacityStart = capacityNow
	t.chargeNow = append(t.chargeNow, chargeNow)
	testing.ContextLogf(ctx, "charge_now value at start: %f, capacity value at start: %f", chargeNow, capacityNow)

	async.Run(ctx, func(ctx context.Context) {
		ticker := time.NewTicker(batteryCheckInterval)
		defer ticker.Stop()

		tOld := time.Now() // last collecting time.
		for {
			select {
			case <-t.collecting:
				close(t.collectingErr)
				return
			case <-ticker.C:
				watt, err := power.ReadSystemPower(ctx, t.batteryPath)
				if err != nil {
					t.collectingErr <- errors.Wrapf(err, "failed to read system power from %q", t.batteryPath)
					return
				}
				tNew := time.Now()
				t.energy += watt * tNew.Sub(tOld).Seconds()
				tOld = tNew

				chargeLeft, err := power.ReadBatteryProperty(ctx, t.batteryPath, "charge_now")
				if err != nil {
					t.collectingErr <- errors.Wrapf(err, "failed to read system property from %q", t.batteryPath)
					return
				}

				t.chargeNow = append(t.chargeNow, chargeLeft)
				t.power = append(t.power, watt)
				t.powerTime = append(t.powerTime, time.Since(timeZero).Seconds())
			case <-ctx.Done():
				t.collectingErr <- ctx.Err()
				return
			}
		}
	}, "BatteryInfoTracker")

	return nil
}

// Stop indicates that the battery tracking should stop. It sets the batteryChargeEnd value.
func (t *BatteryInfoTracker) Stop(ctx context.Context) error {
	if t == nil {
		return nil
	}

	if t.collecting == nil {
		return errors.New("not started")
	}

	if t.trackStartTime.IsZero() {
		return errors.New("Battery info tracker has not started")
	}

	chargeNow, err := power.ReadBatteryProperty(ctx, t.batteryPath, "charge_now")
	if err != nil {
		return err
	}
	capacityNow, err := power.ReadBatteryCapacity(ctx, t.batteryPath)
	if err != nil {
		return err
	}

	t.batteryChargeEnd = chargeNow
	t.batteryCapacityEnd = capacityNow
	t.chargeNow = append(t.chargeNow, chargeNow)
	t.duration = time.Since(t.trackStartTime)
	t.trackStartTime = time.Time{} // Reset to zero for next start.
	testing.ContextLogf(ctx, "charge_now value at end: %f, capacity value at end: %f", chargeNow, capacityNow)

	t.energyFullDesign = t.chargeFullDesign * t.voltageMinDesign * 1e-12 * 3600

	// Stop energy collecting go routine.
	close(t.collecting)
	select {
	case err := <-t.collectingErr:
		if err != nil {
			// On boards like `drallion`, power.ReadSystemPower(ctx) could occasionally
			// fail. Record the error to skip reporting battery info for such boards.
			testing.ContextLog(ctx, "Energe collecting routine returned error: ", err)
			testing.ContextLog(ctx, "Battery info will not be reported")
			t.err = err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// Record stores the collected data into pv for further processing.
func (t *BatteryInfoTracker) Record(pv *perf.Values) {
	if t == nil || t.err != nil {
		return
	}
	pv.Set(perf.Metric{
		Name:      t.prefix + "Battery.Charge.usage",
		Unit:      "microAh",
		Direction: perf.SmallerIsBetter,
	}, t.batteryChargeStart-t.batteryChargeEnd)
	pv.Set(perf.Metric{
		Name:      t.prefix + "Battery.Charge.fullDesign",
		Unit:      "microAh",
		Direction: perf.SmallerIsBetter,
	}, t.chargeFullDesign)
	pv.Set(perf.Metric{
		Name:      t.prefix + "Battery.Voltage.minDesign",
		Unit:      "microV",
		Direction: perf.SmallerIsBetter,
	}, t.voltageMinDesign)
	if t.chargeFullDesign != 0 {
		pv.Set(perf.Metric{
			Name:      t.prefix + "Battery.Charge.usagePercentage",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, (t.batteryChargeStart-t.batteryChargeEnd)/t.chargeFullDesign*100)
	}
	pv.Set(perf.Metric{
		Name:      t.prefix + "Power.usage",
		Unit:      "J",
		Direction: perf.SmallerIsBetter,
	}, t.energy)
	if t.energyFullDesign != 0 {
		// Energy utilization percentage should be at the same level of
		// charge usage percentage. These two indicators can be cross checked.
		pv.Set(perf.Metric{
			Name:      t.prefix + "Power.usagePercentage",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, t.energy/t.energyFullDesign*100)
		pv.Set(perf.Metric{
			Name:      t.prefix + "Power.usagePercentage2",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, t.energy/(t.energyFullDesign*(1-t.lowBatteryShutdownPercent/100))*100)
	}
	pv.Set(perf.Metric{
		Name:      "Power.MinutesBatteryLifeTested",
		Unit:      "minute",
		Direction: perf.SmallerIsBetter,
	}, t.duration.Minutes())
	if (t.batteryChargeStart - t.batteryChargeEnd) > 0 {
		var MinutesBatteryLife float64
		batSizeScale := 1 - t.lowBatteryShutdownPercent/100
		// For longer tests, use charge to calculate battery life since it is not affected by battery voltage swing.
		// For shorter tests, energy is a better estimate since energy is more granular.
		if t.duration.Minutes() >= 60 || t.energy <= 0 {
			chargeUsed := t.batteryChargeStart - t.batteryChargeEnd
			chargeRate := chargeUsed / t.duration.Minutes()
			MinutesBatteryLife = t.chargeFullDesign * batSizeScale / chargeRate
		} else {
			energyRate := t.energy / t.duration.Minutes()
			MinutesBatteryLife = t.energyFullDesign * batSizeScale / energyRate
		}
		if t.lowBatteryShutdownTime > 0 {
			MinutesBatteryLife -= float64(t.lowBatteryShutdownTime) / 60
		}
		pv.Set(perf.Metric{
			Name:      "Power.MinutesBatteryLife",
			Unit:      "minute",
			Direction: perf.BiggerIsBetter,
		}, MinutesBatteryLife)
	}
	pv.Set(perf.Metric{
		Name:      t.prefix + "Battery.Capacity.change",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, t.batteryCapacityStart-t.batteryCapacityEnd)

	powerTimesName := t.prefix + "Power.Timeline.t"
	pv.Set(perf.Metric{
		Name:     powerTimesName,
		Unit:     "s",
		Multiple: true,
	}, t.powerTime...)

	pv.Set(perf.Metric{
		Name:      t.prefix + "Power.Timeline",
		Unit:      "watt",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  powerTimesName,
	}, t.power...)
	pv.Set(perf.Metric{
		Name:      "Power.BatteryRemainingCharge",
		Unit:      "mAh",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, t.chargeNow...)
}
