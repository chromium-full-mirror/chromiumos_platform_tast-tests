// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ping

import (
	"context"
	"os/exec"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// expectPingSuccess pings |addr| as |user|, and returns nil if ping succeeded.
// Note that this function does not dump network info on failure. Use
// ExpectPingSuccessWithTimeout() instead when applicable.
func expectPingSuccess(ctx context.Context, addr, user string) error {
	if err := deletePingEntriesInConntrack(ctx); err != nil {
		return errors.Wrap(err, "failed to reset conntrack before pinging")
	}
	pr := NewLocalRunnerWithoutLogOnError()
	// Only ping once, continuous pings will be very likely to be affected by the
	// connection pinging so it does not make sense. In the routing tests, all the
	// ping targets are in the DUT, so use a small timeout value here.
	res, err := pr.Ping(ctx, addr, ping.Count(1), ping.User(user), ping.MaxDuration(2*time.Second))
	if err != nil {
		return err
	}
	if res.Received == 0 {
		return errors.New("no response received")
	}
	return nil
}

// ExpectPingSuccessWithTimeout keeps pinging |addr| as |user| with |timeout|,
// and returns nil if ping succeeds. |timeout|=0 means only ping once and
// return. Dumps the current network info on failure.
func ExpectPingSuccessWithTimeout(ctx context.Context, addr, user string, timeout time.Duration) (retErr error) {
	defer func() {
		if retErr != nil {
			dumpNetworkInfoAfterPingFailure(ctx)
		}
	}()

	testing.ContextLogf(ctx, "Verifying ping success to %s as user %s", addr, user)
	if timeout == 0 {
		return expectPingSuccess(ctx, addr, user)
	}

	numRetries := 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		err := expectPingSuccess(ctx, addr, user)
		if err == nil {
			return nil
		}
		numRetries++
		return errors.Wrapf(err, "failed to ping with %d retries", numRetries)
	}, &testing.PollOptions{Timeout: timeout, Interval: 500 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to ping with polling")
	}
	return nil
}

// ExpectPingFailure pings |addr| as |user|, and returns nil if ping failed.
// Dumps the current network info on failure.
func ExpectPingFailure(ctx context.Context, addr, user string) (retErr error) {
	defer func() {
		if retErr != nil {
			dumpNetworkInfoAfterPingFailure(ctx)
		}
	}()

	if err := deletePingEntriesInConntrack(ctx); err != nil {
		return errors.Wrap(err, "failed to reset conntrack before pinging")
	}
	testing.ContextLogf(ctx, "Verifying ping failure to %s as user %s", addr, user)
	pr := NewLocalRunnerWithoutLogOnError()
	// Only ping once, continuous pings will be very likely to be affected by the
	// connection pinging so it does not make sense. In the routing tests, all the
	// ping targets are in the DUT, so use a small timeout value here.
	res, err := pr.Ping(ctx, addr, ping.Count(1), ping.User(user), ping.MaxDuration(2*time.Second))
	if err != nil {
		// An error definitely means a ping failure.
		return nil
	}
	if res.Received != 0 {
		return errors.New("received ping reply but not expected")
	}
	return nil
}

func dumpNetworkInfoAfterPingFailure(ctx context.Context) {
	// Encode current time into filename to avoid confliction.
	filename := "network_dump_ping_" + time.Now().Format("030405000") + ".txt"
	if err := dumputil.DumpNetworkInfo(ctx, filename); err != nil {
		testing.ContextLog(ctx, "Failed to dump network info after a ping expectation failure: ", err)
	}
	testing.ContextLog(ctx, "Ping expectation failed. Dumped current network info into ", filename)
}

// deletePingEntriesInConntrack removes all the ping entries in conntrack table
// to avoid ping being affected by connection pinning. This should be called
// before each ping attempt, otherwise ping packets may go to a wrong interface
// due to connection pinning, which is not desired in routing tests.
func deletePingEntriesInConntrack(ctx context.Context) error {
	// `conntrack -D` will exit with 1 if no entry is deleted, so we ignore this
	// case when checking err.
	checkRunError := func(err error) error {
		if err == nil {
			return nil
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				return nil
			}
		}
		return err
	}
	if err := testexec.CommandContext(ctx, "conntrack", "-D", "-f", "ipv4", "-p", "icmp").Run(); checkRunError(err) != nil {
		return errors.Wrap(err, "failed to delete IPv4 ICMP entries in conntrack table")
	}
	if err := testexec.CommandContext(ctx, "conntrack", "-D", "-f", "ipv6", "-p", "icmpv6").Run(); checkRunError(err) != nil {
		return errors.Wrap(err, "failed to delete IPv6 ICMPv6 entries in conntrack table")
	}
	return nil
}

// VerifyInternetConnectivity keeps pinging gstatic.com with |timeout| to verify
// the Internet connectivity as user chronos. Return nil if ping succeeds.
func VerifyInternetConnectivity(ctx context.Context, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return expectPingSuccess(ctx, "gstatic.com", "chronos")
	}, &testing.PollOptions{Timeout: timeout, Interval: 200 * time.Millisecond}); err != nil {
		return errors.Wrapf(err, "failed to reach gstatic.com within %s", timeout)
	}
	return nil
}
