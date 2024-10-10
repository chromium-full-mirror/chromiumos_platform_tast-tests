// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// The retry parameters are set based on similar utilities in autotest.
// https://crsrc.org/o/src/third_party/autotest/files/client/cros/power/power_status.py;l=2492;drc=86c306f49f9da0d7a8d345828725298d5fd606d6
const (
	retryAttempts  = 3
	ecCommandDelay = 2 * time.Second
)

// SetFanMax sets fans to max duty cycle.
func SetFanMax(ctx context.Context) error {
	return util.RunCommandWithRetry(ctx,
		retryAttempts,
		ecCommandDelay,
		"unable to set fan to max duty cycle using ectool",
		"ectool", "fanduty", "100")
}

// SetFanAuto allows the fan to be controlled by the DUT.
func SetFanAuto(ctx context.Context) error {
	return util.RunCommandWithRetry(ctx,
		retryAttempts,
		ecCommandDelay,
		"unable to set fan to auto using ectool",
		"ectool", "autofanctrl")
}

// KeepFanMax will keep the fans running at maximum duty cycle until it is
// stopped by invoking the cleanup function, which will set fan speed to auto.
func KeepFanMax(ctx context.Context, useFan bool) (<-chan error, func(context.Context) error) {
	// For checking if there are any errors occurred while keeping fans at max.
	status := make(chan error, 1)
	// For stopping this go routine.
	stop := make(chan bool, 1)
	// For indicating if the go routine has finished.
	finished := make(chan bool, 1)

	// Do nothing if we don't want to or can't use fan.
	if !useFan || SetFanAuto(ctx) != nil {
		return status, func(context.Context) error { return nil }
	}

	// Always send errors (or nil if no error) to status and signal finished on
	// return.
	go func() {
		for {
			select {
			case <-stop:
				status <- nil
				finished <- true
				return
			default:
				if err := SetFanMax(ctx); err != nil {
					status <- errors.Wrap(err, "failing to control fan while keeping fan max")
					finished <- true
					return
				}
				// GoBigSleepLint: We need to periodically set the fan to max to
				// overwrite the device fan profile.
				if err := testing.Sleep(ctx, 5*time.Second); err != nil {
					status <- errors.Wrap(err, "failing to sleep while keeping fan max")
					finished <- true
					return
				}
			}
		}
	}()

	cleanup := func(ctx context.Context) error {
		// Signal and wait for goroutine to finish.
		stop <- true
		<-finished

		if err := SetFanAuto(ctx); err != nil {
			return errors.Wrap(err, "failing to cleanup after keeping fan max")
		}

		return nil
	}

	return status, cleanup
}
