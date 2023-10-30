// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for remote tests.
package graphics

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	graphics_common "go.chromium.org/tast-tests/cros/common/graphics"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	rebootSetup    = 1 // always reboot the DUT when fixture starts.
	rebootReset    = 2 // always reboot the DUT between tests.
	rebootTearDown = 4 // always reboot the DUT when fixture ends.
)

var rebootLevelVar = testing.RegisterVarString(
	"graphics.rebootLevel",
	"0",
	"If set, reboot the machine based on the reboot level defined in remote/graphics/fixture.go.",
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.GpuRemoteWatcher,
		Desc:            "Handles reboot and misc operation for cleanup the dut state",
		Contacts:        []string{"chromeos-gfx@google.com", "ddmail@google.com"},
		Impl:            &gpuRemoteWatcherImpl{},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    1 * time.Minute,
	})
}

type gpuRemoteWatcherImpl struct {
	d               *dut.DUT
	rebootRequested bool // If set, reboot the machine in TearDown phase.
	rebootLevel     int64
}

func (i *gpuRemoteWatcherImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	i.d = s.DUT()
	level, err := strconv.ParseInt(rebootLevelVar.Value(), 10, 64)
	if err != nil {
		s.Fatal("Failed to parse graphics.rebootLevel: ", err)
	}
	i.rebootLevel = level

	// Delete stale reboot request file.
	if err := i.d.Conn().CommandContext(ctx, "stat", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err == nil {
		s.Log("Detected stale reboot request file on DUT")
		if err := i.d.Conn().CommandContext(ctx, "rm", "-f", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err != nil {
			s.Log("Failed to delete the reboot request file")
		}
	}

	// Do not reboot if rebootRequested is set as it indicates Reset stage has already rebooted the DUT and fixture is reconstructing.
	if (i.rebootLevel&rebootSetup) != 0 && !i.rebootRequested {
		if i.rebootRequested {
		}
		if err := i.d.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot the DUT")
		}
	}

	// We have to set it to false specifically, as SetUp may be called multiple times after we reboot.
	i.rebootRequested = false
	return nil
}

func (i *gpuRemoteWatcherImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if i.rebootRequested || (i.rebootLevel&rebootTearDown) != 0 {
		s.Log("Detected reboot request, rebooting DUT")
		if err := i.d.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot the DUT")
		}
		// Deleting the GraphicsRemoteWatcherRebootFile here in case the reboot does not work out or in case in the future /tmp/ is not memory backed.
		if err := i.d.Conn().CommandContext(ctx, "rm", "-f", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err != nil {
			s.Fatal("Failed to remove reboot request file: ", err)
		}
	}
}

func (i *gpuRemoteWatcherImpl) Reset(ctx context.Context) error {
	// Return an error in Reset stage would leads the fixture to go to TearDown phase and allow all dependent fixtures to recreate.
	if (i.rebootLevel & rebootReset) != 0 {
		i.rebootRequested = true
		return errors.New("rebootLevel set to reboot the machine between tests")
	}
	if err := i.d.Conn().CommandContext(ctx, "stat", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err == nil {
		i.rebootRequested = true
		return errors.New("detected reboot request")
	}
	return nil
}

func (i *gpuRemoteWatcherImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (i *gpuRemoteWatcherImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}
