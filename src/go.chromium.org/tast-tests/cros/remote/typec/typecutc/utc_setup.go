// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecutc contains fixtures and setup utilities for utc device testing.
package typecutc

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/usbutils/utc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestSetupData contains the data needed for utc test setup.
type TestSetupData struct {
	UsbChannel       utc.UsbChannel  // Desired USB channel (e.g., USB2, USB3+2).
	InitialPdState   utc.InitPdState // Desired initial PD state (e.g., DFP, UFP, DRP).
	InitialPowerRole utc.PowerRole   // Desired initial Power Role (e.g., SRC, SNK).
	InitialPdoCount  int64               // Desired initial PDO count (e.g., 1, 2, 3, 4), 0 means not set.
	Flipped          bool                // Whether the Type-C connection should be flipped (use CC2).
	PortNum          int                 // utc port to use (e.g., 0 or 1).
}

// SetupUtc configures the utc device based on the provided TestSetupData.
// It sets the active port, USB channel, initial PD state, power role, and cable orientation.
func SetupUtc(ctx context.Context, ug *utc.UsbTester, setup TestSetupData) error {
	testing.ContextLogf(ctx, "Setting up utc: Port %d, Channel %s, PDState %s, PowerRole %s, Flipped %t",
		setup.PortNum, setup.UsbChannel, setup.InitialPdState, setup.InitialPowerRole, setup.Flipped)

	// Set active utc port.
	if err := ug.SetTestPort(ctx, setup.PortNum); err != nil {
		return errors.Wrapf(err, "failed to set utc active port to %d", setup.PortNum)
	}
	testing.ContextLogf(ctx, "utc active port set to %d", setup.PortNum)

	// Set USB channel (connection mode), if specified.
	if setup.UsbChannel != utc.UsbChannelNotSet {
		if err := ug.SetUsbChannel(ctx, setup.UsbChannel); err != nil {
			return errors.Wrapf(err, "failed to set utc USB channel to %s", setup.UsbChannel)
		}
		testing.ContextLogf(ctx, "utc USB channel set to %s", setup.UsbChannel)
	}

	// Set initial PDO count, if specified.
	if setup.InitialPdoCount != 0 {
		if err := ug.SetSrcPdoCount(ctx, setup.InitialPdoCount); err != nil {
			return errors.Wrapf(err, "failed to set utc initial PDO count to %d", setup.InitialPdoCount)
		}
		testing.ContextLogf(ctx, "utc initial PDO count set to %d", setup.InitialPdoCount)
	}

	// Set initial PD state, if specified.
	if setup.InitialPdState != utc.InitPdStateNotSet {
		if err := ug.SetInitPdState(ctx, setup.InitialPdState); err != nil {
			return errors.Wrapf(err, "failed to set utc initial PD state to %s", setup.InitialPdState)
		}
		testing.ContextLogf(ctx, "utc initial PD state set to %s", setup.InitialPdState)
	}

	// Set initial Power Role, if specified.
	if setup.InitialPowerRole != utc.PowerRoleNotSet {
		if err := ug.SetPowerRole(ctx, setup.InitialPowerRole); err != nil {
			return errors.Wrapf(err, "failed to set utc power role to %s", setup.InitialPowerRole)
		}
		testing.ContextLogf(ctx, "utc power role set to %s", setup.InitialPowerRole)
	}

	// TODO(b/434628173) Unblock flipped tests once the bug is fixed.

	// Replug the utc to trigger negotiation.
	testing.ContextLog(ctx, "Replugging utc")
	if err := ug.Replug(ctx); err != nil {
		return errors.Wrap(err, "failed to replug utc")
	}

	return nil
}
