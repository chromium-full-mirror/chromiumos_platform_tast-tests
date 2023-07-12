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

// Minimum visible value in scaled percentages.
const minVisibleNonlinearPercent = 10.0

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

	prevBrightnessLevel, err := keyboardBrightnessLevel(ctx)
	if err != nil {
		return nil, err
	}

	levelToSet, err := keyboardBrightnessHoverALSLux(ctx, lux)
	if err != nil {
		return nil, errors.Wrap(err, "unable to determine keyboard backlight level for the test")
	}

	minVisibleLevel, err := nonlinearToLevel(ctx, minVisibleNonlinearPercent)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get the min visible level")
	}
	// For most boards, nonlinear/UI percentage is scaled linearly with the
	// full range of keyboard backlight pwm, usually from min valid level(0) to
	// max valid level(100). And usually levelToSet is >= minVisibleLevel.
	// However, when only part of the pwm range is used on some boards, such
	// as kohaku and nightfury, levelToSet ends up smaller than minVisibleLevel.
	// KB backlight will be totally off. This doesn't align with user behavior.
	// Set KB backlight to minVisiblelevel to at least reflect some reasonable
	// power consumption. We are not implementing a more complex solution as
	// kohaku and nightfury are the only boards with this issue.
	if levelToSet < minVisibleLevel {
		testing.ContextLog(ctx, "Set KB nonlinear percentage to be min visible level as the target level is too low")
		levelToSet = minVisibleNonlinearPercent
	}

	testing.ContextLogf(ctx, "Setting keyboard backlight brightness to level %d (%d lux) from level %d", levelToSet, lux, prevBrightnessLevel)
	if err := setKeyboardBrightnessLevel(ctx, levelToSet); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Restoring keyboard backlight brightness to level %d", prevBrightnessLevel)
		return setKeyboardBrightnessLevel(ctx, prevBrightnessLevel)
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

	levelToSet := defaultLevel

	hasALS, err := hasALS(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "light sensor checking error")
	}

	hasHover, err := hasHover(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "hover checking error")
	}

	if hasALS && hasHover {
		levelToSet = (30 * defaultLevel) / 100
	} else if hasALS {
		levelToSet = (40 * defaultLevel) / 100
	} else if hasHover {
		return 0, errors.New("device with hover but no light sensor shouldn't exist")
	}

	return levelToSet, nil
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

func nonlinearToLevel(ctx context.Context, percent float64) (uint, error) {
	kbArg := "--keyboard"
	percentArg := fmt.Sprintf("--nonlinear_to_level=%f", percent)

	output, err := testexec.CommandContext(ctx, "backlight_tool", kbArg, percentArg).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "unable to convert nonlinear to nonlinear percentage")
	}

	level, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to parse current nonlinear percentage from %q", output)
	}

	return uint(level), nil
}

// hasHover checks if hover is detected on the device.
func hasHover(ctx context.Context) (bool, error) {
	_, err := testexec.CommandContext(ctx, "check_powerd_config", "--hover_detection").CombinedOutput()

	exitCode, ok := testexec.ExitCode(err)
	if !ok {
		return false, errors.New("failed to extract exit code from hover checking command")
	}
	if exitCode != 0 {
		testing.ContextLog(ctx, "Checking hover(for KB brightness setting): not present")
		return false, nil
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
		testing.ContextLog(ctx, "Checking ALS(for KB brightness setting): not present")
		return false, nil
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
