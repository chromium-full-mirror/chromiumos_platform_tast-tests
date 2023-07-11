// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package charge provides necessary functionality to perform battery charge, drain on DUT.
package charge

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// EnsureBatteryWithinRange ensures the device battery within the specified min & max percentages.
// Powerd reports battery_percent & battery_display_percent and for all the operations the latter
// one has been taken into consideration.
// This function queries the current battery display percentage of DUT and initiates charging or draining as required.
func EnsureBatteryWithinRange(ctx context.Context, cr *chrome.Chrome, minPercentage, maxPercentage float64) error {
	if minPercentage < 0.0 || minPercentage > 100.0 {
		return errors.New("invalid min percentage, it should be within [0.0, 100.0]")
	}
	if maxPercentage < 0.0 || maxPercentage > 100.0 {
		return errors.New("invalid max percentage, it should be within [0.0, 100.0]")
	}

	status, err := power.GetStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to obtain DUT power status")
	}
	// Battery already is in required range.
	if p := status.BatteryDisplayPercent; p > minPercentage && p < maxPercentage {
		return nil
	}

	if status.BatteryDisplayPercent < minPercentage { // charging
		if err := charge(ctx, minPercentage); err != nil {
			return err
		}
	} else { // discharging
		if err := drain(ctx, cr, maxPercentage); err != nil {
			return err
		}
	}

	return nil
}

// charge charges the device to the specified display percentage.
func charge(ctx context.Context, displayPercentage float64) error {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := setup.WaitUntilPowerSourceChanges(ctx, true); err != nil {
		return err
	}

	// Dimming DUT screen.
	pm, err := power.NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a PowerManager object")
	}
	brightness, err := pm.GetScreenBrightnessPercent(ctx)
	if err := pm.SetScreenBrightness(ctx, 10); err != nil {
		return errors.Wrap(err, "failed to update screen brightness")
	}

	defer func(ctx context.Context) {
		if err := pm.SetScreenBrightness(ctx, brightness); err != nil {
			testing.ContextLogf(ctx, "Failed to reset screen brightness to %.2f%%: %v", brightness, err)
		}
	}(cleanupCtx)

	if err := testing.Poll(ctx, func(context.Context) error {
		status, err := power.GetStatus(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain DUT power status"))
		}
		if status.BatteryDischarging {
			return testing.PollBreak(errors.Wrap(err, "power line isn't connected, can't charge device"))
		}

		if status.BatteryDisplayPercent < displayPercentage {
			return errors.Errorf("still charging from %.2f%% to %.2f%%", status.BatteryDisplayPercent, displayPercentage)
		}
		return nil
	}, &testing.PollOptions{
		Interval: time.Second,
	}); err != nil {
		return errors.Wrapf(err, "failed to charge battery to %.2f%%", displayPercentage)
	}

	return nil
}

const maxForceDischargeErrors = 5

// drain discharges the device battery to the specified display percentage by rendering a resource heavy WebGL graphics.
func drain(ctx context.Context, cr *chrome.Chrome, desiredPercentage float64) error {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Maxing out screen brightness to drain faster.
	pm, err := power.NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a PowerManager object")
	}
	brightness, err := pm.GetScreenBrightnessPercent(ctx)
	if err := pm.SetScreenBrightness(ctx, 100); err != nil {
		return errors.Wrap(err, "failed to update screen brightness")
	}

	defer func(ctx context.Context) {
		if err := pm.SetScreenBrightness(ctx, brightness); err != nil {
			testing.ContextLogf(ctx, "Failed to reset screen brightness to %.2f%%: %v", brightness, err)
		}
	}(cleanupCtx)

	sup, cleanup := setup.New("charge control")
	defer func(ctx context.Context) {
		if err := cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Cleanup failed: ", err)
		}
	}(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Chrome Test API Connection")
	}

	status, err := power.GetStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get power supply status")
	}

	// This forces battery discharge and prevents the display from turning off
	// or dimming.
	sup.Add(setup.PowerTest(ctx, tconn, setup.PowerTestOptions{
		Powerd:    setup.DisablePowerd,
		Backlight: setup.DoNotChangeBacklight,
		DPTF:      setup.DoNotChangeDPTF,
	}, setup.NewBatteryDischarge(true, true, status.BatteryDisplayPercent-desiredPercentage)))
	if err := sup.Check(ctx); err != nil {
		return errors.Wrap(err, "setup failed to setup battery drain conditions")
	}

	if err := setup.WaitUntilPowerSourceChanges(ctx, false); err != nil {
		return err
	}

	// Rendering a WebGL website to consume power quickly.
	conn, err := cr.NewConn(ctx, "https://crospower.page.link/power_BatteryDrain")
	if err != nil {
		testing.ContextLog(ctx, "Failed to open page: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	forceDischargeErrors := 0
	if err := testing.Poll(ctx, func(context.Context) error {
		status, err := power.GetStatus(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain DUT power status"))
		}
		if status.IsLinePowerConnected() {
			forceDischargeErrors++
			// We can discard the cleanup callback since setup will take care of that.
			if _, err = setup.SetBatteryDischarge(ctx, 5.0); err != nil {
				testing.ContextLog(ctx, "Failed to reset force discharge with error: ", err)
			}
			if forceDischargeErrors > maxForceDischargeErrors {
				return testing.PollBreak(errors.New("maximum number of force discharge errors reached"))
			}
		}

		if status.BatteryDisplayPercent > desiredPercentage {
			return errors.Errorf("still discharging from %.2f%% to %.2f%%", status.BatteryDisplayPercent, desiredPercentage)
		}
		return nil
	}, &testing.PollOptions{
		Interval: time.Second,
	}); err != nil {
		return errors.Wrapf(err, "failed to drain battery to %.2f%%", desiredPercentage)
	}

	testing.ContextLogf(ctx, "Succeeded in draining battery with %d force discharge errors", forceDischargeErrors)

	return nil
}
