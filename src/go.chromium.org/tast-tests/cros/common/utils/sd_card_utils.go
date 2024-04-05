// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PlugSDCardViaServo perform microSD plug via servo.
func PlugSDCardViaServo(ctx context.Context, pxy *servo.Proxy) error {
	if err := pxy.Servo().SetString(ctx, "sd_en", "on"); err != nil {
		return errors.Wrap(err, "failed to set sd_en to on")
	}
	if err := pxy.Servo().SetString(ctx, "sd_pwr_en", "on"); err != nil {
		return errors.Wrap(err, "failed to set sd_pwr_en to on")
	}
	if err := pxy.Servo().SetString(ctx, "sd_mux_sel", "dut_sees_usbkey"); err != nil {
		return errors.Wrap(err, "failed to set sd_mux_sel to dut_sees_usbkey")
	}
	return nil
}

// UnplugSDCardViaServo perform microSD unplug via servo.
func UnplugSDCardViaServo(ctx context.Context, pxy *servo.Proxy) error {
	if err := pxy.Servo().SetString(ctx, "sd_en", "off"); err != nil {
		return errors.Wrap(err, "failed to set sd_en to off")
	}
	if err := pxy.Servo().SetString(ctx, "sd_pwr_en", "off"); err != nil {
		return errors.Wrap(err, "failed to set sd_pwr_en to off")
	}
	return nil
}

// WaitForSDCardDetection waits for connected microSD card to detect with provided sdCardName.
func WaitForSDCardDetection(ctx context.Context, dut *dut.DUT, sdCardName string) error {
	const mediaRemovablePath = "/media/removable"
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "ls", mediaRemovablePath).Output()
		if err != nil {
			return errors.Wrap(err, "failed to find connect microSD card in ls command")
		}
		if !strings.Contains(string(out), sdCardName) {
			return errors.New("failed to find connected microSD card in ls command")
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second})
}
