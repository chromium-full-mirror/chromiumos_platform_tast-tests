// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"runtime"
	"time"

	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"gonum.org/v1/gonum/stat"
)

// ThermalSteadyStateConfig contains parameters used for cooling down device
// to a thermal steady state.
type ThermalSteadyStateConfig struct {
	// Number of consecutive samples used to compute mean and standard deviation
	// of temperature.
	SampleSize int
	// Sampling interval.
	Interval time.Duration
	// Maximum amount of time allowed for the device to cooldown.
	Timeout time.Duration
	// Maximum standard deviation acceptable from the samples, this determines the
	// strictness of thermal steady state.
	MaxStandardDeviation float64
	// Temperature threshold for cooldown to pass even if it failed to reach
	// the desired thermal steady state.
	MaxTempAtTimeout float64
}

// waitUntilThermalSteadyState waits until the device reaches the specified
// thermal steady state. If the device cannot reach the steady state before
// timeout, it would still PASS if the mean temperature of the samples <= MaxTempAtTimeout.
func waitUntilThermalSteadyState(ctx context.Context, config ThermalSteadyStateConfig) error {
	samples := make([]float64, 0)
	unstableTempFailure := false
	mean := 0.0
	stdDev := 0.0

	err := testing.Poll(ctx, func(context.Context) error {
		unstableTempFailure = false
		temp, _, err := cpu.Temperature(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read CPU temperature"))
		}

		samples = append(samples, float64(temp)/1000)

		if len(samples) < config.SampleSize {
			return errors.New("not enough temperature samples")
		}

		mean, stdDev = stat.MeanStdDev(samples, nil)
		samples = samples[1:]

		testing.ContextLogf(ctx, "Mean temperature is %f with a standard deviation of %f", mean, stdDev)

		if stdDev > config.MaxStandardDeviation {
			unstableTempFailure = true
			return errors.New("temperature is changing more than specified standard deviation")
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  config.Timeout,
		Interval: config.Interval,
	})

	if err != nil {
		if unstableTempFailure && mean <= config.MaxTempAtTimeout {
			testing.ContextLogf(ctx, "Did not reach thermal steady state but %f degrees is cool enough", mean)
			testing.ContextLog(ctx, "Allow this device to pass thermal steady state cooldown")
			return nil
		}
		return errors.Wrap(err, "failed to cooldown to thermal steady state")
	}

	return nil
}

// async makes a blocking function run asynchronously.
func async(ctx context.Context, f func(ctx context.Context) error) <-chan error {
	out := make(chan error)
	go func() {
		out <- f(ctx)
	}()
	return out
}

// CooldownConfig specifies how a device should be cooled down. When an option
// is set to true or a config is set, these procedures will be enabled during
// cooldown.
//
// It is possible to enable steady state and idle temp cooldown together. This
// means that it will cooldown to steady state first and then idle temperature.
//
// In most cases, cooling down to steady state results in cooler temperature
// than idle temperature, allowing idle temp cooldown to pass instantly.
//
// In other cases where it reached steady state with high temperature, idle
// temp cooldown will allow extra time for temperature to drop.
type CooldownConfig struct {
	// Use fans to accelerate cooldown when possible.
	UseFan bool
	// If provided, cooldown to a thermal steady state.
	SteadyStateConfig *ThermalSteadyStateConfig
	// If provided, cooldown to idle temperature.
	IdleTempConfig *cpu.CoolDownConfig
	// Wait for CPU usage to idle.
	CPUIdle bool
	// Wait for CPU package state to idle.
	PackageStateIdle bool
	// Wait for IO to idle.
	IOIdle bool
}

// cooldownProcedure must be asynchronous and indicates completion or error
// using the returned channel. If a procedure is not enabled in CooldownConfig
// it should return nil immediately through the returned channel.
type cooldownProcedure func(context.Context, CooldownConfig) <-chan error

func afterCPUIdle(ctx context.Context, cfg CooldownConfig) <-chan error {
	return async(ctx, func(ctx context.Context) error {
		if !cfg.CPUIdle {
			return nil
		}
		return cpu.WaitUntilIdle(ctx)
	})
}

func afterIOCooldown(ctx context.Context, cfg CooldownConfig) <-chan error {
	return async(ctx, func(ctx context.Context) error {
		if !cfg.IOIdle {
			return nil
		}
		return util.WaitForIOCooldown(ctx)
	})
}

func afterPackageStateIdle(ctx context.Context, cfg CooldownConfig) <-chan error {
	return async(ctx, func(ctx context.Context) error {
		if !cfg.PackageStateIdle {
			return nil
		}
		if arch := runtime.GOARCH; arch != "arm" && arch != "arm64" {
			if err := cpu.WaitUntilPkgStateIdleWithConfig(ctx, cpu.DefaultPkgIdleConfig()); err != nil {
				return errors.Wrap(err, "failed to wait until CPU package c-state is idle")
			}
		}
		return nil
	})
}

func afterThermalSteadyState(ctx context.Context, cfg CooldownConfig) <-chan error {
	return async(ctx, func(ctx context.Context) error {
		if cfg.SteadyStateConfig == nil {
			return nil
		}
		return waitUntilThermalSteadyState(ctx, *cfg.SteadyStateConfig)
	})
}

func afterIdleTemperature(ctx context.Context, cfg CooldownConfig) <-chan error {
	return async(ctx, func(ctx context.Context) error {
		if cfg.IdleTempConfig == nil {
			return nil
		}
		_, err := cpu.WaitUntilCoolDown(ctx, *cfg.IdleTempConfig)
		return err
	})
}

// ConfigurableCooldown cools down device as specified by the CooldownConfig.
func ConfigurableCooldown(ctx context.Context, cfg CooldownConfig) (err error) {
	// Keep fans running at max RPM throughout cooldown.
	fanStatus, resetFan := KeepFanMax(ctx, cfg.UseFan)

	// Reset fans to auto upon return and append error message if it failed.
	defer func() {
		if rErr := resetFan(ctx); err != nil && rErr != nil {
			rErr = errors.Wrap(rErr, "another failure occured while resetting fans")
			err = errors.Join(err, rErr)
		} else if rErr != nil {
			err = rErr
		}
	}()

	// All cooldowns are included, execution depends on cooldownConfig.
	cooldowns := []cooldownProcedure{
		afterThermalSteadyState,
		afterIdleTemperature,
		afterCPUIdle,
		afterPackageStateIdle,
		afterIOCooldown,
	}

	// Perform cooldowns in sequential order.
	for _, c := range cooldowns {
		select {
		case err = <-c(ctx, cfg):
		case err = <-fanStatus:
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// Cooldown ensures the device is cooled down as much as possible,
// including temperature, CPU usage and package state, and IO. It does not
// guarantee the device reaches a specific temperature on completion but does
// ensures the device cannot be cooled down further. It should be used before
// any test setup but not after it, otherwise cooldown may fail due to system is
// not fully idling.
func Cooldown(ctx context.Context) error {
	cfg := CooldownConfig{
		// Accelerate cooldown when possible.
		UseFan: true,
		// Long timeout ensures devices reach steady state, low standard
		// deviation ensures devices cannot be cooled down further, and low
		// temperature threshold at timeout allows high variance device to
		// continue cooldown.
		SteadyStateConfig: &ThermalSteadyStateConfig{
			SampleSize:           60,
			Interval:             time.Second,
			Timeout:              10 * time.Minute,
			MaxStandardDeviation: 1.0,
			MaxTempAtTimeout:     37.5,
		},
		// Cooldown in other areas as well.
		CPUIdle:          true,
		PackageStateIdle: true,
		IOIdle:           true,
	}

	if err := ConfigurableCooldown(ctx, cfg); err != nil {
		return errors.Wrap(err, "failed to power cooldown")
	}

	return nil
}
