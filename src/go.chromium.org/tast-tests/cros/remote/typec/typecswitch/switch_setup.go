// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typecswitch

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestSetupData contains the data needed for the test setup.
type TestSetupData struct {
	ConnectionMode usbswitch.ConnectionMode
	Flipped        bool
	Iterations     int
}

func SetupSwitch(ctx context.Context, sw usbswitch.Switch, testData TestSetupData) error {
	// Set connection mode.
	if err := sw.EnterMode(ctx, testData.ConnectionMode); err != nil {
		return errors.Wrapf(err, "failed to set switch to %s mode during PreTest", testData.ConnectionMode)
	}
	testing.ContextLogf(ctx, "Switch USB mode set to %s", testData.ConnectionMode)

	// Set flipped.
	if err := sw.FlipOrientation(ctx, testData.Flipped); err != nil {
		// It's ok to not fail the non-flipped case, as probably the wrong cable is used.
		if testData.Flipped {
			return errors.Wrap(err, "failed to set switch orientation during PreTest")
		}
	}
	testing.ContextLogf(ctx, "Switch orientation set to %t", testData.Flipped)

	return nil
}
