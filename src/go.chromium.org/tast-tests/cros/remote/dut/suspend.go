// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const remoteCommandTimeout = 3 * time.Second
const reconnectTimeout = 1 * time.Minute

// SuspendDUT suspends the DUT for given seconds.
func SuspendDUT(ctx context.Context, d *dut.DUT, seconds int) error {
	if d == nil {
		return errors.New("DUT is nil")
	}
	// Run the suspend command with a short timeout
	// since this command can block for long time
	// if the network interface of the DUT goes down
	// before the SSH command finishes.
	cmdCtx, cancel := context.WithTimeout(ctx, remoteCommandTimeout)
	defer cancel()
	d.Conn().CommandContext(
		cmdCtx,
		"suspend_stress_test",
		"-c", "1",
		"--suspend_min", strconv.Itoa(seconds),
		"--suspend_max", strconv.Itoa(seconds),
		"--nopm_print_times", "&").Run() // ignore the error

	// Wait given seconds to ensure that the DUT is suspended
	testing.Sleep(ctx, time.Duration(seconds)*time.Second) // GoBigSleepLint: this sleep is needed for ensuring the suspend operation is finished.

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Set a short timeout to the iteration in case of
		// any SSH operations blocking for a long time.
		ctx, cancel := context.WithTimeout(ctx, remoteCommandTimeout)
		defer cancel()
		if err := d.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT after suspend")
		}
		return nil
	}, &testing.PollOptions{Timeout: reconnectTimeout}); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to suspend")
	}
	return nil
}
