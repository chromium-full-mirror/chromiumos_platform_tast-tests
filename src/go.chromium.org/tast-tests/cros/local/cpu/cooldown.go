// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cpu

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SkipCooldownVar is a Tast variable passed as a flag. If true, skips waiting for the device to
// cool down. Recommended for debugging only.
//
// Usage: -var=cpu.Cooldown.skipCooldown=true.
var SkipCooldownVar = testing.RegisterVarString(
	"cpu.Cooldown.skipCooldown",
	"false",
	"Boolean value. If true, skips waiting for the device to cool down. Recommended for debugging only.",
)

// CoolDownMode defines various modes how to do cool down.
type CoolDownMode int

const (
	// CoolDownPreserveUI defines the mode when current Chrome UI is preserved.
	CoolDownPreserveUI CoolDownMode = iota
	// CoolDownStopUI defines the mode when current Chrome UI is stopped in order to get cool down
	// faster. However, in this mode current Chrome state is lost.
	CoolDownStopUI
)

// TemperatureThresholdMode defines various modes of how to decide the temperature threshold
// to cool down to.
type TemperatureThresholdMode int

const (
	// TemperatureThresholdFixed defines a fixed temperature threshold to wait
	// for.
	TemperatureThresholdFixed = iota
	// TemperatureThresholdPerModel means to check for model-specific temperature
	// thresholds, and use those. If there is no model-specific temperature
	// threshold, we use the given fixed threshold.
	TemperatureThresholdPerModel
)

// CoolDownConfig contains the config to wait for the machine to cooldown.
type CoolDownConfig struct {
	PollTimeout  time.Duration
	PollInterval time.Duration
	// TemperatureThresholdMode denotes how the temperature threshold should
	// be chosen.
	TemperatureThresholdMode TemperatureThresholdMode
	// TemperatureThreshold is the threshold for CPU temperature.
	TemperatureThreshold int
	CoolDownMode         CoolDownMode
}

// DefaultCoolDownConfig returns the default config to wait for the machine to cooldown.
func DefaultCoolDownConfig(mode CoolDownMode) CoolDownConfig {
	return CoolDownConfig{
		PollTimeout:              300 * time.Second,
		PollInterval:             2 * time.Second,
		TemperatureThresholdMode: TemperatureThresholdPerModel,
		TemperatureThreshold:     46000,
		CoolDownMode:             mode,
	}
}

// IdleCoolDownConfig returns the config to wait for the machine to cooldown for PowerIdlePerf test.
// This overrides the default config timeout (5 minutes) to reduce test flakes on low-end devices.
// Config also overrides temp (46C) to 44C to prevent increased power draw during initial measurements.
func IdleCoolDownConfig() CoolDownConfig {
	cdConfig := DefaultCoolDownConfig(CoolDownPreserveUI)
	cdConfig.PollTimeout = 9 * time.Minute
	cdConfig.PollInterval = 5 * time.Second
	cdConfig.TemperatureThreshold = 44000
	return cdConfig
}

// Temperature returns the CPU temperature in milli-Celsius units. It
// also returns the name of the thermal zone it chose.
func Temperature(ctx context.Context) (int, string, error) {
	const (
		// thermalZonePath is the path to thermal zone directories.
		thermalZonePath = "/sys/class/thermal/thermal_zone*"
	)

	// Regular expression to match different CPU vendors' thermal sensor types.
	// - Intel: "x86_pkg_temp"
	// - AMD: "acpitz", "acpitz0"
	// - MediaTek: "cpu_thermal", "soc-thermal", "soc_max", "cpu-little0-thermal" (v6.12+)
	// - Qualcomm: "CPU", "cpu0-thermal"
	// Note: On Intel platforms, there will be x86_pkg_temp and TCPU.
	// In this case, 'TCPU' is not used. (See b/406409030#comment9)
	thermalTypeNameReg := regexp.MustCompile("^(x86_pkg_temp|soc-thermal|soc_max|cpu_thermal|cpu0-thermal|cpu-(little|medium|big)[0-9]-thermal|CPU|acpitz[0-9]?)$")

	zonePaths, err := filepath.Glob(thermalZonePath)
	if err != nil || len(zonePaths) == 0 {
		return 0, "", errors.Wrapf(err, "failed to glob %s", thermalZonePath)
	}

	for _, zonePath := range zonePaths {
		b, err := os.ReadFile(filepath.Join(zonePath, "mode"))
		// No need to return on error because mode file doesn't always exist.
		if err == nil && strings.TrimSpace(string(b)) == "disabled" {
			continue
		}

		zoneTypePath := filepath.Join(zonePath, "type")
		b, err = os.ReadFile(zoneTypePath)
		if err != nil {
			return 0, "", errors.Wrapf(err, "failed to read %q", zoneTypePath)
		}
		zoneType := strings.TrimSpace(string(b))
		if !thermalTypeNameReg.MatchString(zoneType) {
			continue
		}

		zoneTempPath := filepath.Join(zonePath, "temp")
		b, err = os.ReadFile(zoneTempPath)
		if err != nil {
			return 0, "", errors.Wrapf(err, "failed to read %q", zoneTempPath)
		}
		zoneTemp, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return 0, "", errors.Wrapf(err, "failed to parse temperature value in %q", zoneTempPath)
		}

		return zoneTemp, zoneType, nil
	}
	return 0, "", errors.New("could not find valid thermal zone to read temperature from")
}

// WaitUntilCoolDown waits until CPU is cooled down and returns the time it took to cool down.
//
// To skip cooldown for debugging, use
// -var=cpu.Cooldown.skipCooldown=true.
func WaitUntilCoolDown(ctx context.Context, config CoolDownConfig) (time.Duration, error) {
	if IsSkipCooldownSet(ctx) {
		return 0, nil
	}

	timeBefore := time.Now()

	threshold, err := temperatureThreshold(ctx, config)
	if err != nil {
		return 0, err
	}

	switch config.CoolDownMode {
	case CoolDownPreserveUI:
	case CoolDownStopUI:
		// Stop UI in order to cool down CPU faster as Chrome is the heaviest process when
		// system is idle.
		if err := upstart.StopJob(ctx, "ui"); err != nil {
			return 0, errors.Wrap(err, "failed to stop ui")
		}
		defer upstart.StartJob(ctx, "ui")
	default:
		return 0, errors.New("invalid cool down mode")
	}

	testing.ContextLog(ctx, "Waiting until CPU is cooled down")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		t, z, err := Temperature(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get CPU temperature"))
		}

		if t > threshold {
			testing.ContextLogf(ctx, "Waiting until %s temperature (%d) falls below %d", z, t, threshold)
			return errors.Errorf("timed out while waiting until %s temperature (%d) falls below %d", z, t, threshold)
		}
		return nil
	}, &testing.PollOptions{Timeout: config.PollTimeout, Interval: config.PollInterval}); err != nil {
		return 0, err
	}

	timeAfter := time.Now()
	duration := timeAfter.Sub(timeBefore)
	testing.ContextLogf(ctx, "CPU is cooled down (took %f seconds)", duration.Seconds())
	return duration, nil
}

// Cooldown thoroughly cools down for power measurement.
//
// To skip cooldown for debugging, use
// -var=cpu.Cooldown.skipCooldown=true.
func Cooldown(ctx context.Context) error {
	if IsSkipCooldownSet(ctx) {
		return nil
	}

	// Wait until CPU is cooled down and idle.
	if _, err := WaitUntilCoolDown(ctx, DefaultCoolDownConfig(CoolDownPreserveUI)); err != nil {
		return errors.Wrap(err, "CPU failed to cool down")
	}
	if err := WaitUntilIdle(ctx); err != nil {
		return errors.Wrap(err, "CPU failed to idle")
	}
	// Usually takes longer than WaitUntilIdle().
	if arch := runtime.GOARCH; arch != "arm" && arch != "arm64" {
		if err := WaitUntilPkgStateIdleWithConfig(ctx, DefaultPkgIdleConfig()); err != nil {
			return errors.Wrap(err, "CPU package c-state failed to idle")
		}
	}
	return nil
}

// IsSkipCooldownSet checks if the skip cooldown flag is set.
func IsSkipCooldownSet(ctx context.Context) bool {
	skipCooldown, err := strconv.ParseBool(SkipCooldownVar.Value())
	if err != nil {
		testing.ContextLog(ctx, errors.Wrapf(err, "failed to parse runtime variable %s as bool. Got %q", SkipCooldownVar.Name(), SkipCooldownVar.Value()))
		return false
	}
	if skipCooldown {
		testing.ContextLog(ctx, "Skipping cooldown")
	}
	return skipCooldown
}

// temperatureThreshold gets the temperatuer threshold given the CoolDownConfig.
func temperatureThreshold(ctx context.Context, config CoolDownConfig) (int, error) {
	switch config.TemperatureThresholdMode {
	case TemperatureThresholdFixed:
		return config.TemperatureThreshold, nil
	case TemperatureThresholdPerModel:
		name, err := crosconfig.Get(ctx, "/", "name")
		if err != nil {
			return 0, errors.Wrap(err, "could not find model name")
		}
		if temperature, ok := modelTemperatureThresholds[name]; ok {
			testing.ContextLogf(ctx, "Using specific temperature threshold %d for model %s", temperature, name)
			return temperature, nil
		}
		testing.ContextLogf(ctx, "No per model temperature threshold, using default of %d on model %s",
			config.TemperatureThreshold, name)
		return config.TemperatureThreshold, nil
	}
	return config.TemperatureThreshold, nil
}

// modelTemperatureThresholds contains thresholds for models which we have
// determined have run particularly hot. In particular, these are set to the
// 95th percentile idle temperature for each model, as determined by the output
// of the power.IdleTemperature test. If the 95th percentile temperature is less
// than the default threshold of 46 degrees, it is not listed in here.
var modelTemperatureThresholds = map[string]int{
	// atlas
	"atlas": 49000,
	// brask
	"gladios": 53000, // based on b/340958936#comment5
	"lisbon":  53000, // based on b/340958936#comment5
	// brya
	"anahera": 51000,
	// dedede
	"beetley":    50000,
	"blipper":    51000,
	"boten":      48000,
	"bugzzy":     47000,
	"cret":       48000,
	"cret360":    48000,
	"drawcia":    50000,
	"drawlat":    50000,
	"drawman":    52000,
	"galith":     54000,
	"galith360":  51000,
	"gallop":     49000,
	"galnat":     47000,
	"galtic":     47000,
	"galtic360":  53000,
	"kracko":     48000,
	"kracko360":  50000,
	"lantis":     51000,
	"magister":   50000,
	"maglet":     48000,
	"maglia":     49000,
	"maglith":    50000,
	"magma":      50000,
	"magneto":    53000,
	"magolor":    48000,
	"magpie":     50000,
	"metaknight": 52000,
	"pasara":     51000,
	"pirette":    52000,
	"pirika":     51000,
	"sasukette":  47000,
	// elm
	"elm": 50673,
	// fizz
	"sion": 48000,
	// hana
	"hana": 52609,
	// keeby
	"gooey":  48000,
	"lalala": 49000,
	// octopus
	"ampton":   47000,
	"bobba360": 47000,
	"foob360":  47000,
	// puff
	"duffy": 48000,
	// rex
	"rex4es":     54000, // based on b/308581508#comment1
	"screebo":    53000, // based on b/337618192#comment6
	"screebo4es": 54000, // based on b/308581508#comment1
	// volteer
	"chronicler": 53000,
	"eldrid":     48000,
	// zork
	"berknip":   53800,
	"dirinboz":  49800,
	"ezkinil":   50800,
	"gumboz":    54000,
	"jelboz":    53800,
	"jelboz360": 53800,
	"vilboz":    47800,
	"vilboz14":  46800,
	"vilboz360": 46800,
}
