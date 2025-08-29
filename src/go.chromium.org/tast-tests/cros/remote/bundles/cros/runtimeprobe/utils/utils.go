// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils provides util functions for runtimeprobe tast.
package utils

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var (
	// probeFunctionWaivedModels are models that are also waived in the
	// runtimeprobe.ProbeFunction test.
	probeFunctionWaivedModels = []string{
		"blacktip360",
		"domilly",
		"wugtrio",
		"wyrdeer",
	}
)

// WaitServiceState waits for a service to be in a specific state.
func WaitServiceState(ctx context.Context, d *dut.DUT, service, state string) error {
	const (
		pollInterval = time.Second
		pollTimeout  = 2 * time.Minute
	)

	testing.ContextLogf(ctx, "Wait for %s to be %s state", service, state)
	return testing.Poll(ctx, func(ctx context.Context) error {
		output, err := d.Conn().CommandContext(ctx, "initctl", "status", service).Output()
		if err != nil {
			return err
		}
		if strings.Contains(string(output), state) {
			return nil
		}
		return errors.Errorf("%s is not %s state", service, state)
	}, &testing.PollOptions{Interval: pollInterval, Timeout: pollTimeout})
}

// RuntimeHWIDRefreshDeps returns a hardware dependency for Runtime HWID refresh
// related tests.
func RuntimeHWIDRefreshDeps(extraDeps ...hwdep.Condition) hwdep.Deps {
	deps := []hwdep.Condition{
		hwdep.RuntimeProbeConfig(),
		hwdep.RuntimeProbeConfigPrivate(false),
	}
	for _, model := range probeFunctionWaivedModels {
		deps = append(deps, hwdep.SkipOnModel(model))
	}
	deps = append(deps, extraDeps...)
	return hwdep.D(deps...)
}
