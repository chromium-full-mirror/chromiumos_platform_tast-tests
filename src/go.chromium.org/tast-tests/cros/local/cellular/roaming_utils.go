// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// SetRoamingPolicy configures the roaming policy
func SetRoamingPolicy(ctx context.Context, allowRoaming, autoConnect bool) (_ func(ctx context.Context) error, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	_, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not find MM dbus object with a valid sim")
	}

	helper, err := NewHelper(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cellular.Helper")
	}

	var cleanups []uiauto.Action

	cleanupAutoConnect, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, autoConnect)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize autoconnect to false")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			cleanupAutoConnect(ctx)
		}
	}(cleanupCtx)
	// TODO(b/365440573): Apply `slices.Reverse` once "golang.org/x/exp/slices" is replaced with "slice" (which needs Golang is upgraded to 1.21 or later).
	// Insert at front to reverse the cleanup order.
	cleanups = append([]uiauto.Action{cleanupAutoConnect}, cleanups...)

	cleanupPolicyAllowRoaming, err := helper.InitDeviceProperty(ctx, shillconst.DevicePropertyCellularPolicyAllowRoaming, allowRoaming)
	if err != nil {
		return nil, errors.Wrap(err, "could not set PolicyAllowRoaming to true")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			cleanupPolicyAllowRoaming(ctx)
		}
	}(cleanupCtx)
	// TODO(b/365440573): Apply `slices.Reverse` once "golang.org/x/exp/slices" is replaced with "slice" (which needs Golang is upgraded to 1.21 or later).
	// Insert at front to reverse the cleanup order.
	cleanups = append([]uiauto.Action{cleanupPolicyAllowRoaming}, cleanups...)

	cleanupAllowRoaming, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyCellularAllowRoaming, allowRoaming)
	if err != nil {
		return nil, errors.Wrap(err, "could not set AllowRoaming property to true")
	}
	// TODO(b/365440573): Apply `slices.Reverse` once "golang.org/x/exp/slices" is replaced with "slice" (which needs Golang is upgraded to 1.21 or later).
	// Insert at front to reverse the cleanup order.
	cleanups = append([]uiauto.Action{cleanupAllowRoaming}, cleanups...)

	return uiauto.Combine("cleanup roaming policies and auto-connect property.", cleanups...), nil
}
