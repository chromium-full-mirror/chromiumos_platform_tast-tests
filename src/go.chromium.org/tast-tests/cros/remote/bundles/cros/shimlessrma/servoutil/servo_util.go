// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servoutil

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/errors"
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
