// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for remote tests.
package graphics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	graphics_common "go.chromium.org/tast-tests/cros/common/graphics"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.GpuRemoteWatcher,
		Desc:            "Handles reboot and misc operation for cleanup the dut state",
		Contacts:        []string{"chromeos-gfx@google.com", "ddmail@google.com"},
		Impl:            &gpuRemoteWatcherImpl{},
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    1 * time.Minute,
	})
}

type gpuRemoteWatcherImpl struct {
	d *dut.DUT
}

func (i *gpuRemoteWatcherImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	i.d = s.DUT()
	return nil
}

func (i *gpuRemoteWatcherImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	dut := s.DUT()
	if err := dut.Conn().CommandContext(ctx, "stat", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err == nil {
		s.Log("Detected reboot request, rebooting DUT")
		if err := dut.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot the DUT")
		}
		// Deleting the GraphicsRemoteWatcherRebootFile here in case the reboot does not work out or in case in the future /tmp/ is not memory backed.
		if err := dut.Conn().CommandContext(ctx, "rm", "-f", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err != nil {
			s.Fatal("Failed to remove reboot request file: ", err)
		}
	}
}

func (i *gpuRemoteWatcherImpl) Reset(ctx context.Context) error {
	// Return an error in Reset stage would leads the fixture to go to TearDown phase and allow all dependent fixtures to recreate.
	if err := i.d.Conn().CommandContext(ctx, "stat", graphics_common.GraphicsRemoteWatcherRebootFile).Run(); err == nil {
		return errors.New("detect reboot request")
	}
	return nil
}

func (i *gpuRemoteWatcherImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (i *gpuRemoteWatcherImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}
