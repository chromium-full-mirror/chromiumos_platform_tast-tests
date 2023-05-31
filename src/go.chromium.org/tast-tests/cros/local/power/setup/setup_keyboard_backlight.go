// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func noKeyboardBrightness(ctx context.Context) bool {
	_, stderr, err := testexec.CommandContext(ctx, "backlight_tool", "--keyboard", "--get_brightness").SeparatedOutput()
	return err != nil && strings.HasPrefix(string(stderr), "No backlight in")
}

func keyboardBrightnessLevel(ctx context.Context) (uint, error) {
	output, err := testexec.CommandContext(ctx, "backlight_tool", "--keyboard", "--get_brightness").Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get current keyboard brightness level")
	}
	brightness, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current keyboard brightness level from %q", output)
	}
	return uint(brightness), nil
}

func setKeyboardBrightnessLevel(ctx context.Context, brightness uint) error {
	brightnessArg := fmt.Sprintf("--set_brightness=%d", brightness)
	if err := testexec.CommandContext(ctx, "backlight_tool", "--keyboard", brightnessArg).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "unable to set keyboard brightness")
	}
	return nil
}

func setKeyboardBrightnessNonlinearPercent(ctx context.Context, percent float64) error {
	brightnessArg := fmt.Sprintf("--nonlinear_to_level=%f", percent)
	output, err := testexec.CommandContext(ctx, "backlight_tool", "--keyboard", brightnessArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "unable to convert nonlinear percent into level for keyboard brightness")
	}
	brightness, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return errors.Wrapf(err, "unable to parse keyboard brightness level from %q", output)
	}
	return setKeyboardBrightnessLevel(ctx, uint(brightness))
}

// SetKeyboardBrightness sets the keyboard brightness to a nonlinear percentage
// if there is a backlight.
func SetKeyboardBrightness(ctx context.Context, percent float64) (CleanupCallback, error) {
	if noKeyboardBrightness(ctx) {
		return nil, nil
	}
	prevBrightness, err := keyboardBrightnessLevel(ctx)
	if err != nil {
		return nil, err
	}

	testing.ContextLogf(ctx, "Setting keyboard backlight brightness to %f nonlinear percent from level %d", percent, prevBrightness)
	if err := setKeyboardBrightnessNonlinearPercent(ctx, percent); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Restoring keyboard backlight brightness to level %d", prevBrightness)
		return setKeyboardBrightnessLevel(ctx, prevBrightness)
	}, nil
}

// SetKbBrightnessHoverALSLux sets the keyboard brightness based on the presence
// of ALS and hover. For power test setup, lux = 0 is passed in as default to
// turn on keyboard backlight.
func SetKbBrightnessHoverALSLux(ctx context.Context, lux uint) (CleanupCallback, error) {
	if noKeyboardBrightness(ctx) {
		return nil, nil
	}

	prevBrightness, err := keyboardBrightnessLevel(ctx)
	if err != nil {
		return nil, err
	}

	levelToSet, err := keyboardBrightnessHoverALSLux(ctx, lux)
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine keyboard backlight level for the test")
	}

	testing.ContextLogf(ctx, "Setting keyboard backlight brightness to level %d (%d lux) from previous level %d", levelToSet, lux, prevBrightness)
	if err := setBacklightBrightness(ctx, levelToSet); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Restoring keyboard backlight brightness to level %d", prevBrightness)
		return setKeyboardBrightnessLevel(ctx, prevBrightness)
	}, nil
}

func keyboardBrightnessHoverALSLux(ctx context.Context, lux uint) (uint, error) {
	// Calculate keyboard backlight level based on light sensor and hover.
	// These values are based on UMA as mentioned in
	// https://bugs.chromium.org/p/chromium/issues/detail?id=603233#c10
	//
	// ALS  | hover | keyboard backlight level
	// ---------------------------------------
	// No   | No    | default
	// ---------------------------------------
	// Yes  | No    | 40% of default
	// --------------------------------------
	// No   | Yes   | System with this configuration does not exist
	// --------------------------------------
	// Yes  | Yes   | 30% of default
	// --------------------------------------
	//
	// Here default is no Ambient Light Sensor, no hover,
	// default always-on brightness level.

	defaultLevel, err := defaultKBBrightness(ctx, lux)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get default Keyboard brightness level")
	}

	level := defaultLevel

	hasALS, err := hasALS(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "light sensor checking error")
	}

	hasHover, err := hasHover(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "hover checking error")
	}

	if hasALS && hasHover {
		level = (30 * defaultLevel) / 100
	} else if hasALS {
		level = (40 * defaultLevel) / 100
	} else if hasHover {
		return 0, errors.New("device with hover but no light sensor shouldn't exist")
	}

	return level, nil
}

// defaultKBBrightness returns the keyboard backlight at a given lux level.
func defaultKBBrightness(ctx context.Context, lux uint) (uint, error) {
	kbArg := "--keyboard"
	luxArg := "--lux=" + strconv.FormatUint(uint64(lux), 10)

	output, err := testexec.CommandContext(ctx, "backlight_tool", kbArg, "--get_initial_brightness", luxArg, "2>/dev/null").Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to get default keyboard brightness level")
	}

	brightness, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current keyboard brightness level from %q", output)
	}

	return uint(brightness), nil
}

// hasHover checks if hover is detected on the device.
func hasHover(ctx context.Context) (bool, error) {
	_, err := testexec.CommandContext(ctx, "check_powerd_config", "--hover_detection").CombinedOutput()

	exitCode, ok := testexec.ExitCode(err)
	if !ok {
		return false, errors.New("failed to extract exit code from hover checking command")
	}

	if exitCode != 0 {
		return false, errors.New("hover checking command failed with non-zero status")
	}

	testing.ContextLog(ctx, "Checking hover(for KB brightness setting): present")
	return true, nil
}

// hasALS checks if there is a light sensor on the board.
func hasALS(ctx context.Context) (bool, error) {
	output, err := testexec.CommandContext(ctx, "check_powerd_config", "--ambient_light_sensor").CombinedOutput()

	exitCode, ok := testexec.ExitCode(err)
	if !ok {
		return false, errors.New("failed to extract exit code from light sensor checking command")
	}

	if exitCode != 0 {
		return false, errors.New("light sensor checking command failed with non-zero status")
	}

	alsNum, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return false, errors.Wrapf(err, "unable to parse light sensor numbers from %q", output)
	}

	if alsNum < 1 {
		testing.ContextLog(ctx, "Checking ALS(for KB brightness setting): not present")
		return false, nil
	}

	testing.ContextLogf(ctx, "Checking ALS(for KB brightness setting): present, has %d sensor(s)", alsNum)
	return true, nil
}
