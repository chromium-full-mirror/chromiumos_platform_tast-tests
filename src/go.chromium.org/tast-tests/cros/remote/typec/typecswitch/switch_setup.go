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

	// TODO(b/434628173) Unblock flipped tests once the bug is fixed.

	return nil
}
