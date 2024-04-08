// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component execution function.
package utils

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DisableServoPower switches the servo's power role from source to sink. Surrounding the power role
// change, the function will disable/enable the servo's USB-A ports to prevent enumeration failure.
func DisableServoPower(ctx context.Context, dut *dut.DUT, svo *servo.Servo) error {
	if err := svo.SetOnOff(ctx, servo.BottomUSBKeyPwr, servo.Off); err != nil {
		return errors.Wrap(err, "failed to turn off servo bottom mux")
	}
	if err := svo.SetOnOff(ctx, servo.TopUSBKeyPwr, servo.Off); err != nil {
		return errors.Wrap(err, "failed to turn off servo top mux")
	}

	// GoBigSleepLint: Allow USB-A devices to disconnect.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "unable to sleep during USB device disconnection")
	}

	if err := svo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		return errors.Wrap(err, "unable to set servo power role to sink")
	}

	// GoBigSleepLint: Wait for the servo to disable power.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "unable to sleep after servo power role change")
	}

	if err := svo.SetOnOff(ctx, servo.BottomUSBKeyPwr, servo.On); err != nil {
		return errors.Wrap(err, "failed to turn on servo bottom mux")
	}
	if err := svo.SetOnOff(ctx, servo.TopUSBKeyPwr, servo.On); err != nil {
		return errors.Wrap(err, "failed to turn on servo top mux")
	}

	return nil
}

// EnableServoPower will restore the servo power role to source and enable the servo's USB-A ports.
func EnableServoPower(ctx context.Context, dut *dut.DUT, svo *servo.Servo) error {
	if err := svo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		return errors.Wrap(err, "unable to set servo power role to source")
	}

	// GoBigSleepLint: Allow servo power role to settle.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "unable to sleep after servo power role change")
	}

	if err := svo.SetOnOff(ctx, servo.BottomUSBKeyPwr, servo.On); err != nil {
		return errors.Wrap(err, "failed to turn on servo bottom mux")
	}
	if err := svo.SetOnOff(ctx, servo.TopUSBKeyPwr, servo.On); err != nil {
		return errors.Wrap(err, "failed to turn on servo top mux")
	}

	return nil
}
