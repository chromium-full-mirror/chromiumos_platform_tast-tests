// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ConnectivityTestParams is the configuration for connectivity tests.
type ConnectivityTestParams struct {
	V6Only bool
}

// NetworkConnectivity tests that a guest is able to connect to the host network.
func NetworkConnectivity(ctx context.Context, guest vm.Guest, v6only bool, testEnv *routing.SimpleNetworkEnv) []error {
	var errs []error

	// Wait for online and verify topology in host.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return append(errs, errors.Wrap(err, "failed to wait for service online"))
	}
	routerAddrs, err := testEnv.Router.WaitForVethInAddrs(ctx, !v6only, true)
	if err != nil {
		return append(errs, errors.Wrap(err, "failed to get inner addrs from router env"))
	}
	serverAddrs, err := testEnv.Server.WaitForVethInAddrs(ctx, !v6only, true)
	if err != nil {
		return append(errs, errors.Wrap(err, "failed to get inner addrs from server env"))
	}
	var pingAddrs []string
	if !v6only {
		pingAddrs = append(pingAddrs, serverAddrs.IPv4Addr.String())
		pingAddrs = append(pingAddrs, routerAddrs.IPv4Addr.String())
	}
	for _, ip := range serverAddrs.IPv6Addrs {
		pingAddrs = append(pingAddrs, ip.String())
	}
	for _, ip := range routerAddrs.IPv6Addrs {
		pingAddrs = append(pingAddrs, ip.String())
	}
	if !v6only {
		pingAddrs = append(pingAddrs, routing.TestDomainNameV4)
	}
	pingAddrs = append(pingAddrs, routing.TestDomainNameV6)
	for _, target := range pingAddrs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 10*time.Second); err != nil {
			return append(errs, errors.Wrapf(err, "network verification failed: %v is not reachable as user %s on host", target, "chronos"))
		}
	}

	// Check if testEnv prefix propagated into the guest, and log it for debugging.
	const addressPollTimeout = 10 * time.Second
	if !v6only {
		if err := checkAddress(ctx, guest, false, addressPollTimeout); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to get IPv4 address in guest"))
		}
	}
	if err := checkAddress(ctx, guest, true, addressPollTimeout); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to get IPv6 address in guest"))
	}

	// Verify reachability to destinations in the guest
	const pingPollTimeout = 20 * time.Second
	for _, ip := range pingAddrs {
		if err := pingWithRetryAndTimeout(ctx, guest, ip, pingPollTimeout); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to ping %s from guest", ip))
		} else {
			testing.ContextLogf(ctx, "Succeeded to ping %s from guest", ip)
		}
	}

	return errs
}

func checkAddress(ctx context.Context, guest vm.Guest, ipv6 bool, timeout time.Duration) error {
	ipCmdOption := "-4"
	versionForLog := "IPv4"
	if ipv6 {
		ipCmdOption = "-6"
		versionForLog = "IPv6"
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := guest.Command(ctx, "ip", ipCmdOption, "addr", "show", "scope", "global").Output(testexec.DumpLogOnError)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return errors.Errorf("no global %s address is configured", versionForLog)
		}
		testing.ContextLog(ctx, "Guest ", versionForLog, " address information: \n", string(out))
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

func pingWithRetryAndTimeout(ctx context.Context, guest vm.Guest, addr string, timeout time.Duration) error {
	numRetries := 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		output, err := guest.Command(ctx, "ping", "-c1", "-w2", addr).Output()
		if err == nil {
			return nil
		}
		numRetries++
		return errors.Wrapf(err, "failed to ping %s with %d retries: output %s", addr, numRetries, output)
	}, &testing.PollOptions{Timeout: timeout, Interval: 500 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to ping with polling")
	}
	return nil
}
