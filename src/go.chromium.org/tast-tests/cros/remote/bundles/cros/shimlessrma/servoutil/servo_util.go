// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servoutil

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// A BatteryStateValue is a string accepted by the bpforce control.
type BatteryStateValue string

// These are the string values that can be passed to the GSC bpforce control.
const (
	BatteryStateOn     BatteryStateValue = "connect"
	BatteryStateOff    BatteryStateValue = "disconnect"
	BatteryStateFollow BatteryStateValue = "follow_batt_pres"
)

// retryTimes is the maximum number of times the action will be retried.
const retryTimes = 3

// SetBatteryState sets the state of battery via servo.
func SetBatteryState(ctx context.Context, firmwareHelper *firmware.Helper, state BatteryStateValue) error {
	cmd := fmt.Sprintf("bpforce %s atboot", state)

	// Certain devices experience unreliable GSC UART communication, leading to potential character loss and
	// subsequent 'command not found' errors. Thus we add a retry mechanism to address the this.
	if err := action.Retry(retryTimes, func(ctx context.Context) error {
		return firmwareHelper.Servo.CheckGSCCommandOutput(ctx, cmd, []string{"batt pres:"})
	}, 0)(ctx); err != nil {
		return errors.Wrapf(err, "failed to set the battery state to %s ", state)
	}

	return nil
}

// RebootGSC reboots GSC via console.
func RebootGSC(ctx context.Context, firmwareHelper *firmware.Helper) error {
	// Certain devices experience unreliable GSC UART communication, leading to potential character loss and
	// subsequent 'command not found' errors. Thus we add a retry mechanism to address the this.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := firmwareHelper.Servo.RunGSCCommand(ctx, "reboot"); err != nil {
			return errors.Wrap(err, "failed to run gsc command")
		}
		return nil
	}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to reboot GSC")
	}

	return nil
}

// IsTi50 checks if the device uses Ti50 GSC firmware.
func IsTi50(ctx context.Context, firmwareHelper *firmware.Helper) (bool, error) {
	versionInfo, err := firmwareHelper.Servo.GSCVersionInfo(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get GSC version info")
	}

	return versionInfo.IsTi50, nil
}
