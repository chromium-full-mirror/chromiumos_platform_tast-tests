// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"chromiumos/tast/common/testexec"
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
