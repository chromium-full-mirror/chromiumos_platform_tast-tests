// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// BootIDChecker is a helper to retrieve and compare boot id.
type BootIDChecker struct {
	dut        *dut.DUT
	lastBootID string
}

// FatalIfBootIDChanged is a helper function to fail the test if boot id changed.
// The function is a short hand to allow for deferring the check in an eloquent way.
func FatalIfBootIDChanged(ctx context.Context, b *BootIDChecker, s *testing.State) {
	if err := b.ErrorIfChanged(ctx); err != nil {
		s.Fatal("Unexpected reboot: ", err)
	}
}

// NewBootIDChecker creates new boot id checker helper.
// It fails the test if the object can not be created.
func NewBootIDChecker(ctx context.Context, dut *dut.DUT, s *testing.State) *BootIDChecker {
	b := &BootIDChecker{dut: dut}
	id, err := b.retrieveBootID(ctx)
	if err != nil {
		s.Fatal("Can't create boot id checker: ", err)
	}
	b.lastBootID = id
	return b
}

// ErrorIfChanged returns an error if boot id of the device has changed.
func (b *BootIDChecker) ErrorIfChanged(ctx context.Context) error {
	id, err := b.retrieveBootID(ctx)
	if err != nil {
		return errors.Wrap(err, "can't compare boot id")
	}

	if id != b.lastBootID {
		return errors.Wrapf(err, "boot id changed, was: %q, now: %q", b.lastBootID, id)
	}

	return nil
}

func (b *BootIDChecker) retrieveBootID(ctx context.Context) (string, error) {
	id, err := RunCmdWithStringOutput(ctx, b.dut, "cat", "/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", errors.Wrap(err, "can't read boot id")
	}

	return id, nil
}
