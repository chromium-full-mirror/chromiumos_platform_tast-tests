// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"gonum.org/v1/gonum/stat"
)

// ThermalCooldownParams contains parameters for thermally cooling down device.
type ThermalCooldownParams struct {
	// Number of consecutive samples used to check the cooldown progress.
	SampleCount int
	// Sampling interval.
	Interval time.Duration
	// Maximum amount of time allowed for the device to cooldown.
	Timeout time.Duration
	// Maximum deviation acceptable from the samples.
	MaxStandardDeviation float64
}

// ThermalCooldown tries to cooldown DUT to a steady state quickly by
// setting fans to 100% and wait until the temperature meets the
// stopping criteria defined in ThermalCooldownParams.
func ThermalCooldown(ctx context.Context, p ThermalCooldownParams) error {
	// The retry parameters are set based on similar utilities in autotest.
	// crsrc.org/o/src/third_party/autotest/files/client/cros/power/power_status.py;l=2492
	const (
		retryAttempts  = 3
		ecCommandDelay = 2 * time.Second
	)

	setFanMaxDuty := func(ctx context.Context) error {
		return util.RunCommandWithRetry(ctx,
			retryAttempts,
			ecCommandDelay,
			"unable to set fan to max duty cycle using ectool",
			"ectool", "fanduty", "100")
	}

	setFanAutoCtrl := func(ctx context.Context) error {
		return util.RunCommandWithRetry(ctx,
			retryAttempts,
			ecCommandDelay,
			"unable to set fan to auto using using ectool",
			"ectool", "autofanctrl")
	}

	// Some fanless devices return error when setting fan speed. Avoid setting
	// fan speed for these devices. For fanless devices which do not return
	// error, setting fan speed would make no functional difference.
	useFanForCooldown := setFanAutoCtrl(ctx) == nil

	samples := make([]float64, 0)

	if err := testing.Poll(ctx, func(context.Context) error {
		if useFanForCooldown {
			if err := setFanMaxDuty(ctx); err != nil {
				return err
			}
		}

		temp, _, err := cpu.Temperature(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read CPU temperature"))
		}

		samples = append(samples, float64(temp)/1000)

		if len(samples) < p.SampleCount {
			return errors.Wrap(err, "not enough temperature samples")
		}

		mean, stdDev := stat.MeanStdDev(samples, nil)
		samples = samples[1:]

		testing.ContextLogf(ctx, "Cooling down. Average temperature is %f with a standard deviation of %f", mean, stdDev)

		if stdDev > p.MaxStandardDeviation {
			return errors.Wrap(err, "temperature is changing more than specified standard deviation")
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  p.Timeout,
		Interval: p.Interval,
	}); err != nil {
		err = errors.Wrap(err, "failed to cooldown")

		if useFanForCooldown {
			if fanErr := setFanAutoCtrl(ctx); fanErr != nil {
				fanErr = errors.Wrap(fanErr, "also failed to reset fan to auto")
				err = errors.Join(err, fanErr)
			}
		}

		return err
	}

	if useFanForCooldown {
		if err := setFanAutoCtrl(ctx); err != nil {
			return err
		}
	}

	if err := Cooldown(ctx); err != nil {
		return err
	}

	return nil
}
