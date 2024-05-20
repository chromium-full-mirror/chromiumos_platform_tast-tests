// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diag is a library of common functionality to utilize the
// DUT network diagnostic.
package diag

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	pingTimeout               = 2 * time.Second
	verifyConnectivityTimeout = 10 * time.Second
)

// DUTConnectionCheck establishes a connection to Google's public DNS server and CDN
// server to determine whether the device can access the network.
func DUTConnectionCheck(ctx context.Context) error {
	testing.ContextLog(ctx, "Verifying DUT connection to Internet")
	if err := testexec.CommandContext(ctx, "ping", "-c", "1", "-w", strings.TrimSuffix(pingTimeout.String(), "s"), "8.8.8.8").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "DUT connections verification failed: Google public IP is not reachable")
	}
	if err := ping.VerifyInternetConnectivity(ctx, verifyConnectivityTimeout); err != nil {
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
