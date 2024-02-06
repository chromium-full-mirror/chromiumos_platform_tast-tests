// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements functions to interact with servo to perform
// power related operations.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
        "go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
        fullBatteryPercent     = 95.0
        fullChargePollTimeout  = 110 * time.Minute
        dischargePollTimeout   = 60 * time.Minute
        chargePollInterval     = 1 * time.Second
)

// BootDutViaPowerPress performs power button normal press to power on DUT via servo.
func BootDutViaPowerPress(ctx context.Context, h *Helper, dut *dut.DUT) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return errors.Wrap(err, "failed to power normal press")
		}
		if err := dut.WaitConnect(waitCtx); err != nil {
			return errors.Wrap(err, "failed to wait connect DUT")
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Minute})
}

func TestChargingVoltagesAfterDischarge(ctx context.Context, h *Helper, percentBattDischargeLevel float64) error {
	battery, err := GetECBatteryStatus(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get ec battery state")
	}

	if battery.Charge > percentBattDischargeLevel {
		testing.ContextLog(ctx, "Disconnecting charger")
		if err := PollToSetChargerStatus(ctx, h, false); err != nil {
			return errors.Wrap(err, "failed to disconnect charger")
		}

		// As the firmware test with bootModeNormal does not receive
		// browser services on its initialization, we cannot easily
		// use Chrome for battery drain procedure. Instead, we can
		// simply spawn stress-ng (which seems to be available in
		// base rootfs) for specified amount of time.
		// See also battery_service.go:DrainBattery

		script := "cd /usr/local/bin; stress-ng --cpu 32 --timeout 1m"

		testing.ContextLog(ctx, "Initiating battery discharging")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			testing.ContextLog(ctx, "Stressing CPU to discharge battery")
			cmd := h.DUT.Conn().CommandContext(ctx, "sh", "-c", script)
			if out, err := cmd.Output(ssh.DumpLogOnError); err != nil {
				err := errors.Wrapf(err, "failed to discharge battery using CPU stress, got output: %v", string(out))
				testing.PollBreak(err)
			}

			battery, err := GetECBatteryStatus(ctx, h)
			if err != nil {
				testing.ContextLog(ctx, "Failed to get battery state: ", err)
				return errors.Wrap(err, "failed to get battery state")
			}
			if battery.Charge > percentBattDischargeLevel {
				testing.ContextLogf(ctx, "Current charge: %v, target: %v", battery.Charge, percentBattDischargeLevel)
				return errors.Errorf("Not enough battery discharged: %v, want %v", battery.Charge, percentBattDischargeLevel)
			}

			return nil
			// poll at 1s since the stress script will block progress for 1 minute
		}, &testing.PollOptions{Timeout: dischargePollTimeout, Interval: chargePollInterval}); err != nil {
			return errors.Wrap(err, "failed to discharge battery")
		}
	}

	testing.ContextLog(ctx, "Reconnecting charger")
	if err := PollToSetChargerStatus(ctx, h, true); err != nil {
		return errors.Wrap(err, "failed to connect charger")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := CheckChargingState(ctx, h); err != nil {
			return errors.Wrap(err, "failed to verify expected charging voltages")
		}

		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}

