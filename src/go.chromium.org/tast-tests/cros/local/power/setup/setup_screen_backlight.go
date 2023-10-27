// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// backlightBrightness returns the current backlight brightness in level.
func backlightBrightness(ctx context.Context) (uint, error) {
	output, err := testexec.CommandContext(ctx, "backlight_tool", "--get_brightness").Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get current backlight brightness")
	}
	brightness, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current backlight brightness from %q", output)
	}
	return uint(brightness), nil
}

// defaultBacklightBrightness returns the backlight brightness at a given lux
// level and a given force battery condition. We use backlight_tool instead of
// sysfs directly because the conversion between lux and brightness is complicated
// and hard to extract.
func defaultBacklightBrightness(ctx context.Context, lux uint, forceBattery bool) (uint, error) {
	var batteryArg string
	if forceBattery {
		batteryArg = "--force_battery"
	}
	luxArg := "--lux=" + strconv.FormatUint(uint64(lux), 10)

	output, err := testexec.CommandContext(ctx, "backlight_tool", "--get_initial_brightness", luxArg, batteryArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get default backlight brightness")
	}
	brightness, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "unable to parse default backlight brightness")
	}
	return uint(brightness), nil
}

// setBacklightBrightness sets the backlight brightness level.
func setBacklightBrightness(ctx context.Context, brightness uint) error {
	brightnessArg := "--set_brightness=" + strconv.FormatUint(uint64(brightness), 10)
	err := testexec.CommandContext(ctx, "backlight_tool", brightnessArg).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "unable to set backlight brightness")
	}
	return nil
}

// setBacklightBrightnessLinearPercent sets the backlight brightness linear
// percentage.
func setBacklightBrightnessLinearPercent(ctx context.Context, percent float64) error {
	brightnessArg := "--set_brightness_percent=" + strconv.FormatFloat(percent, 'f', -1, 64)
	err := testexec.CommandContext(ctx, "backlight_tool", brightnessArg).Run(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "unable to set backlight brightness")
	}
	return nil
}

// listBacklightPaths lists paths of backlights in sysfs.
func listBacklightPaths() ([]string, error) {
	const sysfsBacklightPath = "/sys/class/backlight"
	files, err := ioutil.ReadDir(sysfsBacklightPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Ignore NotExist error since /sys/class/backlight may not exist.
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to read sysfs dir")
	}
	var backlightPaths []string
	for _, file := range files {
		devPath := path.Join(sysfsBacklightPath, file.Name())
		backlightPaths = append(backlightPaths, devPath)
	}
	return backlightPaths, nil
}

// checkBacklightExists returns whether display backlights exist.
func checkBacklightExists(ctx context.Context) (bool, error) {
	backlightPaths, err := listBacklightPaths()
	if err != nil {
		return false, err
	}
	numBacklights := len(backlightPaths)
	// TODO(hikalium): Remove listing backlights after checking this logic works
	// on all platforms.
	testing.ContextLogf(ctx, "%v backlights found: %s", numBacklights, strings.Join(backlightPaths[:], ","))
	return numBacklights != 0, nil
}

// SetBacklightLux sets the screen backlight to a brightness in lux, and returns
// a callback to restore backlight brightness after test finishes.
func SetBacklightLux(ctx context.Context, lux uint, forceBattery bool) (CleanupCallback, error) {
	backlightExists, err := checkBacklightExists(ctx)
	if err != nil {
		return nil, err
	}
	if !backlightExists {
		testing.ContextLog(ctx, "Skipping setting screen backlight brightness since there are no backlights")
		return nil, nil
	}
	prevBrightness, err := backlightBrightness(ctx)
	if err != nil {
		return nil, err
	}

	brightness, err := defaultBacklightBrightness(ctx, lux, forceBattery)
	if err != nil {
		return nil, err
	}
	testing.ContextLogf(ctx, "Setting screen backlight brightness to level %d (%d lux) from level %d", brightness, lux, prevBrightness)
	if err := setBacklightBrightness(ctx, brightness); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Restoring screen backlight brightness to level %d", prevBrightness)
		return setBacklightBrightness(ctx, prevBrightness)
	}, nil
}

// SetBacklightBrightnessLinearPercent sets the backlight brightness linear
// percentage, and returns a callback to restore backlight brightness after test
// finishes.
func SetBacklightBrightnessLinearPercent(ctx context.Context, percent float64) (CleanupCallback, error) {
	backlightExists, err := checkBacklightExists(ctx)
	if err != nil {
		return nil, err
	}
	if !backlightExists {
		testing.ContextLog(ctx, "Skipping setting screen backlight brightness since there are no backlights")
		return nil, nil
	}
	prevBrightness, err := backlightBrightness(ctx)
	if err != nil {
		return nil, err
	}

	testing.ContextLogf(ctx, "Setting screen backlight brightness to %.2f%% linear percent from level %d", percent, prevBrightness)
	if err := setBacklightBrightnessLinearPercent(ctx, percent); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Restoring screen backlight brightness to level %d", prevBrightness)
		return setBacklightBrightness(ctx, prevBrightness)
	}, nil
}

// levelToLinear converts the provided brightness level to linear.
func levelToLinear(ctx context.Context, level uint) (float64, error) {
	levelArg := fmt.Sprintf("--level_to_linear=%d", level)

	output, err := testexec.CommandContext(ctx, "backlight_tool", levelArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to convert level to linear percentage")
	}

	percent, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current linear percentage from %q", output)
	}

	return percent, nil
}

// levelToNonLinear converts the provided brightness level to nonlinear.
func levelToNonLinear(ctx context.Context, level uint) (float64, error) {
	levelArg := fmt.Sprintf("--level_to_nonlinear=%d", level)

	output, err := testexec.CommandContext(ctx, "backlight_tool", levelArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to convert level to nonlinear percentage")
	}

	percent, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current nonlinear percentage from %q", output)
	}

	return float64(percent), nil
}

// linearToNonlinear converts the provided linear brightness percent to nonlinear.
func linearToNonlinear(ctx context.Context, percent float64) (float64, error) {
	percentArg := fmt.Sprintf("--linear_to_nonlinear=%f", percent)

	output, err := testexec.CommandContext(ctx, "backlight_tool", percentArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to convert linear to nonlinear percentage")
	}

	nonlinear, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current nonlinear percentage from %q", output)
	}

	return nonlinear, nil
}

// nonlinearToLinear converts the provided nonlinear brightness percent to linear.
func nonlinearToLinear(ctx context.Context, percent float64) (float64, error) {
	percentArg := fmt.Sprintf("--nonlinear_to_linear=%f", percent)

	output, err := testexec.CommandContext(ctx, "backlight_tool", percentArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to convert linear to nonlinear percentage")
	}

	linear, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current nonlinear percentage from %q", output)
	}

	return linear, nil
}

// LinearBacklight returns the current screen brightness percentage.
func LinearBacklight(ctx context.Context) (float64, error) {
	output, err := testexec.CommandContext(ctx, "backlight_tool", "--get_brightness_percent").Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get current brightness percent")
	}

	brightness, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current brightness from %q", output)
	}

	return brightness, nil
}

// SetBacklightBrightnessNonLinearPercent sets the backlight brightness nonlinear percentage.
func SetBacklightBrightnessNonLinearPercent(ctx context.Context, nonlinear float64) error {
	linear, err := nonlinearToLinear(ctx, nonlinear)
	if err != nil {
		return errors.Wrap(err, "failed to convert nonlinear percentage to linear")
	}

	setBacklightBrightnessLinearPercent(ctx, linear)

	return nil
}

// DefaultNonliearPercent returns default nonlinear percentage.
func DefaultNonliearPercent(ctx context.Context) (float64, error) {
	level, err := defaultBacklightBrightness(ctx, 150, true)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get default level")
	}

	nonlinear, err := levelToNonLinear(ctx, level)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert from level to nonlinear")
	}

	return nonlinear, nil
}
