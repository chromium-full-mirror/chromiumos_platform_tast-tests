// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diag is a library of common functionality to utilize the
// DUT network diagnostic.
package diag

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DUTConnectionCheck establishes a connection to Google's CDN
// server to determine whether the device can access the network.
func DUTConnectionCheck(ctx context.Context, timeout time.Duration) error {
	testing.ContextLog(ctx, "Verifying DUT connection to Internet")
	if err := ping.VerifyInternetConnectivity(ctx, timeout); err != nil {
		return errors.Wrap(err, "DUT connections verification failed: Google CDN server is not reachable or DNS resolution failed")
	}
	return nil
}

// DUTConnectionResolve tries to resolve DUT connections by restart shill process.
func DUTConnectionResolve(ctx context.Context) error {
	testing.ContextLog(ctx, "Trying to resolve network connections")
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to lock the check network hook")
	}
	defer unlock()

	if errs := shill.ResetShill(ctx); len(errs) != 0 {
		for _, err := range errs {
			testing.ContextLog(ctx, "ResetShill error: ", err)
		}
		return errors.New("failed resetting shill")
	}
	return nil
}
