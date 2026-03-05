// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/remote/kdump"
)

func init() {
	addHook(&Hook{
		Name:         "kdumpHook",
		Desc:         "Enable kdump feature to generate additional dumps for kernel crashes",
		Contacts:     []string{"chromeos-platform-stability-team@google.com", "jasongustaman@google.com"},
		BugComponent: "b:1672909",
		Impl:         &kdumpHook{},
	})
}

var kdumpEnable = testing.RegisterVarString(
	"hooks.kdump.enable",
	"",
	"A variable to decide whether kdump feature should be enabled",
)

type kdumpHook struct {
	dut     *dut.DUT
	cleanup func(ctx context.Context) error
}

// isKdumpSupported checks if kdump feature is supported by the device.
// Kdump is supported on devices with x86 architecture (b/451780016).
func isKdumpSupported(ctx context.Context, d *dut.DUT) (bool, error) {
	arch, err := d.Conn().CommandContext(ctx, "uname", "-m").Output()
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(string(arch), "x86"), nil
}

// SetUp enables kdump if the corresponding var is set.
func (h *kdumpHook) SetUp(ctx context.Context, s *HookState) error {
	if kdumpEnable.Value() != "true" {
		return nil
	}
	h.dut = s.DUT()
	if supported, err := isKdumpSupported(ctx, h.dut); err != nil {
		return errors.Wrap(err, "failed to check whether kdump is supported")
	} else if !supported {
		return nil
	}
	cleanup, err := kdump.EnableKdump(ctx, h.dut)
	if err != nil {
		return errors.Wrap(err, "failed to enable kdump")
	}
	h.cleanup = cleanup
	return nil
}

// Reset checks the status of kdump and re-enables it if it is disabled.
func (h *kdumpHook) Reset(ctx context.Context) error {
	if h.cleanup == nil {
		return nil
	}
	// Only the first cleanup is necessary. It is expected for the cleanup
	// to revert to the state of kdump before the test runs.
	if _, err := kdump.EnableKdump(ctx, h.dut); err != nil {
		return errors.Wrap(err, "failed to enable kdump")
	}
	return nil
}

// PreTest does nothing.
func (h *kdumpHook) PreTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// PostTest does nothing.
func (h *kdumpHook) PostTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// TearDown disables kdump if it was previously enabled by the fixture setup.
func (h *kdumpHook) TearDown(ctx context.Context, s *HookState) error {
	if h.cleanup == nil {
		return nil
	}
	if err := h.cleanup(ctx); err != nil {
		return errors.Wrap(err, "failed to disable kdump")
	}
	return nil
}
