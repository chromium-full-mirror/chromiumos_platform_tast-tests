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
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ConnectionCheckTimeout is the initial timeout waiting for a network connection.
	ConnectionCheckTimeout = 120 * time.Second

	// CheckAttemptTimeout is the timeout of a single attempt of waiting for
	// a network connection.
	CheckAttemptTimeout = 5 * time.Second

	// ResolveCheckTimeout is the timeout waiting for a network connection
	// following an attempt to resolve the network issue.
	ResolveCheckTimeout = 30 * time.Second
)

// DUTConnectionCheck establishes a connection to Google's CDN
// server to determine whether the device can access the network.
func DUTConnectionCheck(ctx context.Context, timeout time.Duration) error {
	testing.ContextLog(ctx, "Verifying DUT connection to Internet")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ping.VerifyInternetConnectivity(ctx, CheckAttemptTimeout); err != nil {
			return errors.Wrap(err, "no DUT internet connectivity")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "DUT connections verification failed: Google CDN server is not reachable or DNS resolution failed")
	}
	testing.ContextLog(ctx, "DUT connection to Internet: OK")
	return nil
}

// DUTConnectionCheckAndDump checks the DUT connection and dumps network info if it fails.
func DUTConnectionCheckAndDump(ctx context.Context, timeout time.Duration) error {
	err := DUTConnectionCheck(ctx, timeout)
	if err == nil {
		return nil
	}
	dumpfile := "network_dump_ping_" + time.Now().Format("030405000") + ".txt"
	if err := dumputil.DumpNetworkInfo(ctx, dumpfile); err != nil {
		testing.ContextLog(ctx, "Failed to dump network info after a ping expectation failure: ", err)
	}
	testing.ContextLog(ctx, "Current network info dumped into ", dumpfile)

	return err
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

// DUTNetworkCheckAndResolve doing network diagnostics to debug DUT connection issues.
// If there is any issue, try to fix it at first, and return an error if the fix attempt fails.
func DUTNetworkCheckAndResolve(ctx context.Context) error {
	if err := DUTConnectionCheck(ctx, ConnectionCheckTimeout); err != nil {
		if err := DUTConnectionResolve(ctx); err != nil {
			return err
		}
		if err := DUTConnectionCheckAndDump(ctx, ResolveCheckTimeout); err != nil {
			return errors.Wrap(err, "DUT network connections still failed after shill reset")
		}
		testing.ContextLog(ctx, "DUT network connections fixed")
	}
	return nil
}
