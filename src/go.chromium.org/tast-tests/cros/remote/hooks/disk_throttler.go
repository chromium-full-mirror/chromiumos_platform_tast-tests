// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hooks

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	util "go.chromium.org/tast-tests/cros/remote/storage/diskthrottler"
)

func init() {
	addHook(&Hook{
		Name:         "diskThrottler",
		Desc:         "Limit performance of root storage device",
		Contacts:     []string{"chromeos-storage@google.com", "dlunev@google.com"},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Impl:         &diskThrottlerHook{},
	})
}

var diskThrottlerModeVar = testing.RegisterVarString(
	"hooks.disk_throttler.mode",
	"",
	"Storage throttling mode. Can be 'ephemeral' - doesn't persist over reboot and 'persistent' - persists over reboot. Unset means disabled.",
)

var diskThrottlerLimitsVar = testing.RegisterVarString(
	"hooks.disk_throttler.limits",
	"",
	"Throttling profile, format '<r_iops_limit>,<w_iops_limit>,<r_bps_limit>,<w_bps_limit>'. If any field is set to 0 - it means no limit for the dimension.",
)

type diskThrottlerHook struct {
	shouldRun         bool
	isPersistent      bool
	dut               *dut.DUT
	rootdevPath       string
	rootdevMajorMinor string
	readIopsLimit     int64
	writeIopsLimit    int64
	readBpsLimit      int64
	writeBpsLimit     int64
}

// Hook interface

// SetUp will parse the runtime variables hooks.disk_throttler.mode and
// hooks.disk_throttler.limits. If they are set, Setup will set persistent or
// ephemeral (depending on arguments) limits for internal storage performance.
// If any limits were set on device before start of the hook, they are not
// preserved. This hook will not be executed if hooks.disk_throttler.mode is not
// set.
func (h *diskThrottlerHook) SetUp(ctx context.Context, s *HookState) error {
	err := h.parseVariables()
	if err != nil {
		return errors.Wrap(err, "error while parsing disk throttler configuration")
	}

	if !h.shouldRun {
		return nil
	}

	h.dut = s.DUT()

	err = h.populateRootdev(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to populate rootdev")
	}

	if err := h.setLimits(ctx); err != nil {
		return errors.Wrap(err, "failed to set limits")
	}

	return nil
}

// Reset re-asserts the ephemeral limits.
func (h *diskThrottlerHook) Reset(ctx context.Context) error {
	if h.shouldRun && !h.isPersistent {
		if err := h.setLimits(ctx); err != nil {
			return errors.Wrap(err, "failed to re-assert ephemeral limits")
		}
	}

	return nil
}

// PreTest does nothing.
func (h *diskThrottlerHook) PreTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// PostTest does nothing.
func (h *diskThrottlerHook) PostTest(ctx context.Context, s *HookTestState) error {
	return nil
}

// TearDown removes the previously set limits.
func (h *diskThrottlerHook) TearDown(ctx context.Context, s *HookState) error {
	if !h.shouldRun {
		return nil
	}

	return h.resetLimits(ctx)
}

// Supporting functions

func (h *diskThrottlerHook) parseVariables() error {
	h.shouldRun = false
	h.isPersistent = false
	h.readIopsLimit = 0
	h.writeIopsLimit = 0
	h.readBpsLimit = 0
	h.writeBpsLimit = 0

	if diskThrottlerModeVar.Value() == "" {
		return nil
	}

	h.shouldRun = true
	if diskThrottlerModeVar.Value() == "persistent" {
		h.isPersistent = true
	} else if diskThrottlerModeVar.Value() != "ephemeral" {
		return errors.Errorf("unrecognized disk throttling mode: %s", diskThrottlerModeVar.Value())
	}

	limits := strings.Split(diskThrottlerLimitsVar.Value(), ",")
	if len(limits) != 4 {
		return errors.New("'hooks.disk_throttler.limits' must be set as 4 comma-separated non-negative int values, e.g. '1,2,3,4'")
	}

	var err error

	h.readIopsLimit, err = util.ParseLimit(limits[0])
	if err != nil {
		return errors.Wrap(err, "malformed read iops limit")
	}

	h.writeIopsLimit, err = util.ParseLimit(limits[1])
	if err != nil {
		return errors.Wrap(err, "malformed write iops limit")
	}

	h.readBpsLimit, err = util.ParseLimit(limits[2])
	if err != nil {
		return errors.Wrap(err, "malformed read bps limit")
	}

	h.writeBpsLimit, err = util.ParseLimit(limits[3])
	if err != nil {
		return errors.Wrap(err, "malformed write bps limit")
	}

	return nil
}

func (h *diskThrottlerHook) populateRootdev(ctx context.Context) error {
	var err error

	h.rootdevPath, err = util.RootdevPath(ctx, h.dut)
	if err != nil {
		return errors.Wrap(err, "failed to get root device")
	}

	h.rootdevMajorMinor, err = util.DevMajorMinor(ctx, h.dut, h.rootdevPath)
	if err != nil {
		return errors.Wrap(err, "failed to stat root device")
	}

	return nil
}

func (h *diskThrottlerHook) setLimits(ctx context.Context) error {
	var err error

	if h.isPersistent {
		err = util.SetPersistentLimits(ctx, h.dut, h.rootdevPath, h.readIopsLimit, h.writeIopsLimit, h.readBpsLimit, h.writeBpsLimit)
	} else {
		err = util.SetEphemeralLimits(ctx, h.dut, h.rootdevMajorMinor, h.readIopsLimit, h.writeIopsLimit, h.readBpsLimit, h.writeBpsLimit)
	}

	if err != nil {
		// Best effort attempt to restore previous state
		_ = h.resetLimits(ctx)
	}

	return err
}

func (h *diskThrottlerHook) resetLimits(ctx context.Context) error {
	if h.isPersistent {
		return util.ResetPersistentLimits(ctx, h.dut, h.rootdevPath)
	}
	return util.ResetEphemeralLimits(ctx, h.dut, h.rootdevMajorMinor)
}
