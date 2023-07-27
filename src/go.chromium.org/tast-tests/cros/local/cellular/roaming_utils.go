// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var uiPollOptions = testing.PollOptions{
	Timeout:  60 * time.Second,
	Interval: 500 * time.Millisecond,
}

// SetRoamingPolicy configures the roaming policy
func SetRoamingPolicy(ctx context.Context, allowRoaming, autoConnect bool) error {
	_, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		return errors.Wrap(err, "could not find MM dbus object with a valid sim")
	}

	helper, err := NewHelper(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create cellular.Helper")
	}

	_, err = helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, autoConnect)
	if err != nil {
		return errors.Wrap(err, "could not initialize autoconnect to false")
	}

	_, err = helper.InitDeviceProperty(ctx, shillconst.DevicePropertyCellularPolicyAllowRoaming, allowRoaming)
	if err != nil {
		return errors.Wrap(err, "could not set PolicyAllowRoaming to true")
	}

	_, err = helper.InitServiceProperty(ctx, shillconst.ServicePropertyCellularAllowRoaming, allowRoaming)
	if err != nil {
		return errors.Wrap(err, "could not set AllowRoaming property to true")
	}

	return nil
}
