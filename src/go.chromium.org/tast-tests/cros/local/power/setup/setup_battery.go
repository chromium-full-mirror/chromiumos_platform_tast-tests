// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type chargeState struct {
	command        string
	state          string
	expectedOutput string
}

var (
	coDontCharge = chargeState{"chargeoverride", "dontcharge", "Override port set to -2"}
	coOff        = chargeState{"chargeoverride", "off", "Override port set to -1"}
	ccDischarge  = chargeState{"chargecontrol", "discharge", "Charge state machine force discharge."}
	ccNormal     = chargeState{"chargecontrol", "normal", "Charge state machine is in normal mode."}
)

const (
	// BatteryPreparationTimeout is the time required to charge and drain battery
	// to specified range.
	BatteryPreparationTimeout = 2 * time.Hour
	// minCapacityChargeTimeout is the time to wait for the battery to be charged
	// to its minimum operating capacity.
	minCapacityChargeTimeout = 20 * time.Minute
)

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
	shutdownCutoff, err := metrics.LowBatteryShutdownPercent(ctx)
	if err != nil {
		return nil, err
	}
	lowBatteryCutoff := shutdownCutoff + expectedMaxCapacityDischarge
	devPath, err := metrics.SysfsBatteryPath(ctx)
	if err != nil {
		return nil, err
	}
	capacity, err := metrics.ReadBatteryCapacity(ctx, devPath)
	if err != nil {
		return nil, err
	}
	energy, err := metrics.ReadBatteryEnergy(ctx, devPath)
	if err != nil {
		return nil, err
	}
	status, err := metrics.ReadBatteryStatus(ctx, devPath)
	if err != nil {
		return nil, err
	}
	if status == metrics.BatteryStatusDischarging {
		testing.ContextLog(ctx, "WARNING Battery is already discharging")
	}

	testing.ContextLog(ctx, "Setting battery to discharge. Current capacity: ", capacity, "% (", energy, "Wh), Shutdown cutoff: ", shutdownCutoff, "%, Expected maximum discharge during test: ", expectedMaxCapacityDischarge, "%")
	if lowBatteryCutoff > capacity {
		return nil, errors.Errorf("battery percent %.2f is too low to start discharging", capacity)
	}

	useChargeOverride := util.SupportChargeOverride()
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
		capacityAfterTest, err := metrics.ReadBatteryCapacity(ctx, devPath)
		if err == nil {
			dischargePercent = capacity - capacityAfterTest
		}
		energyAfterTest, err := metrics.ReadBatteryEnergy(ctx, devPath)
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
	if !util.SupportChargeOverride() {
		chargeState = ccNormal
	}
	return setChargeState(ctx, chargeState)
}

// DisableBatteryCharging will force AC power to be temporarily disconnected to
// prevent battery from being charged. However, this would be overridden when the
// AC power cable is physically re-inserted to the DUT again.
func DisableBatteryCharging(ctx context.Context) error {
	chargeState := coDontCharge
	if !util.SupportChargeOverride() {
		chargeState = ccDischarge
	}
	return setChargeState(ctx, chargeState)
}

// Battery prepares battery to make sure the device has at least 50% of battery.
// It disables all multicast after the preparation and returns a cleanup function.
// TODO(b/432384910, b/445765922): Remove multicast actions after the USB reconnect issue got fixed.
func Battery(ctx context.Context, total time.Duration, discharge bool) (CleanupCallback, error) {
	const (
		testDurationThreshold = 20 * time.Minute
		ethernetDuration      = 5 * time.Second
		ethernetTimeout       = 30 * time.Second
	)

	cleanup := func(context.Context) error { return nil }
	if !discharge {
		return cleanup, nil
	}
	// USB might be temporary disconnected after |PrepareBattery|.
	// Check if the ethernet is connected before applying the actions.
	isEthernetConnected := IsEthernetConnected(ctx)

	// For tests that take more than 20 minutes, make sure the device has at least
	// 50% of battery.
	if total >= testDurationThreshold {
		testing.ContextLogf(ctx, "Prepare the device to have at least %.2f%% battery", power.RegressionTestChargeParam.MinChargePercentage)
		if err := PrepareBattery(ctx, power.RegressionTestChargeParam); err != nil {
			return cleanup, errors.Wrap(err, "failed to prepare battery")
		}
	}
	if !isEthernetConnected {
		return cleanup, nil
	}
	// The USB may not immediately disconnected after preparing the battery.
	// Make sure the ethernet is connected for specified duration before applying the actions.
	if err := ensureEthernetConnectedFor(ctx, ethernetDuration, ethernetTimeout); err != nil {
		return cleanup, errors.Wrap(err, "failed to ensure ethernet is connected")
	}
	return DisableAllMulticast(ctx)

}

// PrepareBattery charges or drains the battery to reach the specified
// range. Upon completion, the DUT would be allowed to resume charging or
// being forced to discharge as specified.
func PrepareBattery(ctx context.Context, cp power.ChargeParams) error {
	if cp.MinChargePercentage < 0.0 || cp.MinChargePercentage > 100.0 {
		return errors.New("invalid min percentage, it should be within [0.0, 100.0]")
	}
	if cp.MaxChargePercentage < 0.0 || cp.MaxChargePercentage > 100.0 {
		return errors.New("invalid max percentage, it should be within [0.0, 100.0]")
	}
	if cp.MaxChargePercentage < cp.MinChargePercentage {
		return errors.New("invalid battery range, max percentage is smaller than min percentage")
	}

	status, err := power.GetStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to obtain DUT power status")
	}

	degradation := status.BatteryChargeFull / status.BatteryChargeFullDesign
	// The charge would take a long time in the capacity interval 80% ~ 100%.
	// Cap the minimum charge percentage to 80% to avoid timeout.
	const minChargeThreshold = 80.0
	minChargePercentage := min(cp.MinChargePercentage/degradation, cp.MaxChargePercentage, minChargeThreshold)
	testing.ContextLogf(ctx, "Degradation of the battery is %f, use %f as minimum charge percentage", degradation, minChargePercentage)

	currentPercentage := status.BatteryPercent
	logStr := "battery"
	if cp.UseDisplayPercentage {
		currentPercentage = status.BatteryDisplayPercent
		logStr = "display battery"
	}
	testing.ContextLogf(ctx, "Current %s charge is %.2f%%", logStr, currentPercentage)
	testing.ContextLogf(ctx, "Acceptable %s range is [%.2f%%, %.2f%%]", logStr, minChargePercentage, cp.MaxChargePercentage)

	batteryPreparationTimeout := BatteryPreparationTimeout
	if cp.MaxBatteryPreparationTime != time.Duration(0) {
		batteryPreparationTimeout = cp.MaxBatteryPreparationTime
	}

	if currentPercentage >= minChargePercentage && currentPercentage <= cp.MaxChargePercentage {
		testing.ContextLog(ctx, "Current battery charge is within the acceptable range")
		err = nil
	} else if currentPercentage < minChargePercentage {
		testing.ContextLog(ctx, "Current battery charge is below the acceptable range")
		err = chargeBattery(ctx, batteryPreparationTimeout, minChargePercentage, cp.IsPowerQual, cp.UseDisplayPercentage)
	} else {
		testing.ContextLog(ctx, "Current battery charge is above the acceptable range")
		err = drainBattery(ctx, batteryPreparationTimeout, cp.MaxChargePercentage, cp.UseDisplayPercentage)
	}

	if err != nil {
		return err
	}

	if cp.DischargeOnCompletion {
		return DisableBatteryCharging(ctx)
	}

	return AllowBatteryCharging(ctx)
}

func chargeBattery(ctx context.Context, batteryPreparationTimeout time.Duration, targetPercentage float64, isPowerQual, UseDisplayPercent bool) error {
	testing.ContextLog(ctx, "Start charging battery")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	screenBrightnessPercentage := screenBrightnessPercentageToSet(ctx, isPowerQual)
	resetScreenBrightness, err := SetBacklightBrightnessLinearPercent(ctx, screenBrightnessPercentage)
	if err != nil {
		return errors.Wrap(err, "failed to set screen backlight brightness")
	}
	if resetScreenBrightness != nil {
		defer resetScreenBrightness(cleanupCtx)
	}

	resetKeyboardBrightness, err := SetKeyboardBrightness(ctx, 0)
	if err != nil {
		return errors.Wrap(err, "failed to minimise keyboard brightness")
	}
	if resetKeyboardBrightness != nil {
		defer resetKeyboardBrightness(cleanupCtx)
	}

	if err := AllowBatteryCharging(ctx); err != nil {
		return errors.Wrap(err, "failed to disable charge override to allow charging on DUT")
	}

	if err := WaitUntilPowerSourceChanges(ctx, true); err != nil {
		return errors.Wrap(err, "timed out while waiting for DUT to start charging")
	}
	return testing.Poll(ctx, func(context.Context) error {
		status, err := power.GetStatus(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain DUT power status"))
		}
		if !power.IsLinePowerConnected(status) {
			return testing.PollBreak(errors.Wrap(err, "power source is not connected while charging"))
		}
		currentPercent := status.BatteryPercent
		logStr := "battery"
		if UseDisplayPercent {
			currentPercent = status.BatteryDisplayPercent
			logStr = "display battery"
		}
		testing.ContextLogf(ctx, "Current %s percentage is %v%%", logStr, currentPercent)
		if currentPercent < targetPercentage {
			return errors.New("failed to reach target battery charge")
		}
		// Some battery status goes directly from "Charging" to "Discharging" when the battery is full
		// to lessen battery degradation, so when the battery is more than 97 and goes to "Discharging"
		// we treat it as the battery is full and stop charging.
		if status.BatteryDischarging && status.BatteryPercent >= 97 {
			testing.ContextLog(ctx, "Battery full(Discharge on AC), stop charging")
			return nil
		}
		testing.ContextLog(ctx, "Successfully charged battery")
		return nil
	}, &testing.PollOptions{
		Interval: time.Minute,
		Timeout:  batteryPreparationTimeout,
	})
}

func drainBattery(ctx context.Context, batteryPreparationTimeout time.Duration, targetPercentage float64, UseDisplayPercent bool) error {
	testing.ContextLog(ctx, "Start draining battery")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	resetScreenBrightness, err := SetBacklightBrightnessLinearPercent(ctx, 100)
	if err != nil {
		return errors.Wrap(err, "failed to maximise screen brightness")
	}
	if resetScreenBrightness != nil {
		defer resetScreenBrightness(cleanupCtx)
	}

	resetKeyboardBrightness, err := SetKeyboardBrightness(ctx, 100)
	if err != nil {
		return errors.Wrap(err, "failed to maximise keyboard brightness")
	}
	if resetKeyboardBrightness != nil {
		defer resetKeyboardBrightness(cleanupCtx)
	}

	if err := DisableBatteryCharging(ctx); err != nil {
		return errors.Wrap(err, "failed to enable charge override to discharge on DUT")
	}

	if err := WaitUntilPowerSourceChanges(ctx, false); err != nil {
		return errors.Wrap(err, "timed out while waiting for DUT to start discharging")
	}

	stopStressTest, err := StressCPU(ctx, runtime.NumCPU(), "/tmp")
	if err != nil {
		return errors.Wrap(err, "unable to start stress-ng to drain battery")
	}
	defer stopStressTest(cleanupCtx)

	return testing.Poll(ctx, func(context.Context) error {
		status, err := power.GetStatus(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain DUT power status"))
		}
		if power.IsLinePowerConnected(status) {
			return testing.PollBreak(errors.Wrap(err, "power source is connected while discharging"))
		}
		currentPercent := status.BatteryPercent
		logStr := "battery"
		if UseDisplayPercent {
			currentPercent = status.BatteryDisplayPercent
			logStr = "display battery"
		}
		testing.ContextLogf(ctx, "Current %s percentage is %v%%", logStr, currentPercent)
		if currentPercent > targetPercentage {
			return errors.New("failed to reach target battery charge")
		}
		testing.ContextLog(ctx, "Successfully drained battery")
		return nil
	}, &testing.PollOptions{
		Interval: time.Second,
		Timeout:  batteryPreparationTimeout,
	})
}

// WaitUntilPowerSourceChanges waits until the power source is changed to the
// specified source.
func WaitUntilPowerSourceChanges(ctx context.Context, acConnected bool) error {
	return testing.Poll(ctx, func(context.Context) error {
		status, err := power.GetStatus(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain DUT power status"))
		}
		if acConnected && !power.IsLinePowerConnected(status) {
			return errors.New("expected device to connect to AC power source but device is discharging from battery")
		}
		if !acConnected && power.IsLinePowerConnected(status) {
			return errors.New("expected device to discharge from battery but device is charging from AC power source")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 20 * time.Second,
	})
}

// StressCPU generates heavy load on a specified number of cores.
func StressCPU(ctx context.Context, nCores int, tempPath string) (CleanupCallback, error) {
	cmd := testexec.CommandContext(ctx, "stress-ng", "--cpu", strconv.Itoa(nCores), "--temp-path", tempPath)
	err := cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "failed to start stress-ng")
	}
	return func(context.Context) error {
		if err := cmd.Kill(); err != nil {
			return errors.Wrap(err, "failed to send SIGKILL to terminate stress-ng process")
		}
		code, ok := testexec.ExitCode(cmd.Wait())
		if !ok /* Exit Code Extraction Status */ {
			return errors.New("stress-ng process failed to exit and exit code cannot be extracted")
		}
		if code != 137 /* SIGKILL */ {
			return errors.Errorf("stress-ng process failed to exit with exit code %v", code)
		}
		return nil
	}, nil
}

func screenBrightnessPercentageToSet(ctx context.Context, isPowerQual bool) float64 {
	if !isPowerQual {
		return 5.0
	}

	defaultBrightnessLevel, err := defaultBacklightBrightness(ctx, 150, false)
	if err != nil {
		testing.ContextLog(ctx, "Use 40.0% linear percentage as the default screen brightness for battery charge qual test for the following error: ", err)
		return 40.0
	}

	defaultBrightnessPercent, err := levelToLinear(ctx, defaultBrightnessLevel)
	if err != nil {
		testing.ContextLog(ctx, "Use 40.0% linear percentage as the default screen brightness for battery charge qual test for the following error: ", err)
		return 40.0
	}

	return defaultBrightnessPercent
}
