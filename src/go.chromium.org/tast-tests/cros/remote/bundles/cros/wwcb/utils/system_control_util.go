// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ShutdownDUT performs power long press to shutdown DUT.
func ShutdownDUT(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
	testing.ContextLog(ctx, "Performing shutdown DUT")
	if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurLongPress); err != nil {
		return errors.Wrap(err, "failed to power long press")
	}
	sdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		return errors.Wrap(err, "failed to wait for unreachable")
	}
	return nil
}

// SuspendDUT peforms DUT suspend with powerd_dbus_suspend command.
func SuspendDUT(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy) error {
	testing.ContextLog(ctx, "Performing suspend DUT")
	return testing.Poll(ctx, func(ctx context.Context) error {
		suspendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := dut.Conn().CommandContext(suspendCtx, "powerd_dbus_suspend", "--disable_dark_resume=false").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return errors.Wrap(err, "failed to execute powerd_dbus_suspend command")
		}

		sCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := dut.WaitUnreachable(sCtx); err != nil {
			return errors.Wrap(err, "failed to wait for unreachable")
		}
		return nil
	}, &testing.PollOptions{Timeout: 40 * time.Second, Interval: time.Second})
}

// PowerOnDUT performs power normal press to wake DUT.
func PowerOnDUT(ctx context.Context, pxy *servo.Proxy, dut *dut.DUT) error {
	testing.ContextLog(ctx, "Performing power on DUT")
	return testing.Poll(ctx, func(ctx context.Context) error {
		waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return errors.Wrap(err, "failed to press the power button of the DUT")
		}
		if err := dut.WaitConnect(waitCtx); err != nil {
			return errors.Wrap(err, "failed to wait connect to the DUT")
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Minute})
}
