// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/testing"
)

const (
	pingTimeout      = 60 * time.Second
	curlTimeout      = 60 * time.Second
	defaultInterval  = 10 * time.Second
	speedtestTimeout = 2 * time.Minute
	googleDotComIPv6 = "ipv6.google.com"
	googleDotComIPv4 = "ipv4.google.com"
)

func verifyCrostiniConnectivityUsingPing(ctx context.Context, binCmd, addr string, cmd func(context.Context, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify connectivity to: ", addr)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, binCmd, "-c1", "-w5", addr).Run(); err != nil {
			testing.ContextLog(ctx, "Failed to ping: ", addr)
			return errors.Wrap(err, "failed ping test in Crostini")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  pingTimeout,
		Interval: defaultInterval,
	}); err != nil {
		return errors.Wrap(err, "failed ping test in Crostini")
	}
	return nil
}

// VerifyCrostiniIPConnectivity verifies the ip connectivity from crostini via cellular interface using ping.
func VerifyCrostiniIPConnectivity(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd, ipv4, ipv6 bool) error {
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	if ipv4 {
		if err := verifyCrostiniConnectivityUsingPing(ctx, "/bin/ping", googleDotComIPv4, cmd); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyCrostiniConnectivityUsingPing(ctx, "/bin/ping6", googleDotComIPv6, cmd); err != nil {
			return err
		}
	}
	return nil
}

func verifyIPConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipType, addr string) error {
	testing.ContextLog(ctx, "Verify IP connectivity using curl to: ", addr)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "curl", ipType, addr).Run(); err != nil {
			return errors.Wrap(err, "failed curl test")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  curlTimeout,
		Interval: defaultInterval,
	}); err != nil {
		return errors.Wrap(err, "failed  curl test")
	}
	return nil
}

// VerifyIPConnectivityUsingCurl verifies the ip connectivity from Host via cellular interface using curl.
func VerifyIPConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, bindir string) error {
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	if ipv4 {
		if err := verifyIPConnectivityUsingCurl(ctx, cmd, "-4", googleDotComIPv4); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyIPConnectivityUsingCurl(ctx, cmd, "-6", googleDotComIPv6); err != nil {
			return err
		}
	}
	return nil
}

func verifyIPConnectivityUsingPing(ctx context.Context, binCmd, addr string, cmd func(context.Context, string, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify IP connectivity to: ", addr)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, binCmd, "-c1", "-w5", addr).Run(); err != nil {
			testing.ContextLog(ctx, "Failed to ping: ", addr)
			return errors.Wrap(err, "failed ping test")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  pingTimeout,
		Interval: defaultInterval,
	}); err != nil {
		return errors.Wrap(err, "failed ping test")
	}
	return nil
}

// VerifyIPConnectivity verifies the ip connectivity from ARC via cellular interface using ping.
func VerifyIPConnectivity(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, bindir string) error {
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	if ipv4 {
		if err := verifyIPConnectivityUsingPing(ctx, filepath.Join(bindir, "ping"), googleDotComIPv4, cmd); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyIPConnectivityUsingPing(ctx, filepath.Join(bindir, "ping6"), googleDotComIPv6, cmd); err != nil {
			return err
		}
	}
	return nil
}

func verifyArcIPConnectivityUsingPing(ctx context.Context, addr string, a *arc.ARC) error {
	testing.ContextLog(ctx, "Verify ARC IP connectivity to: ", addr)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := arc.ExpectPingSuccess(ctx, a, "", addr); err != nil {
			testing.ContextLog(ctx, "Failed to ping: ", addr)
			return errors.Wrap(err, "failed ip ping test")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  120 * time.Second,
		Interval: 30 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed ip ping test")
	}
	return nil
}

// VerifyArcIPConnectivity verifies the ip connectivity from ARC via cellular interface using ping.
func VerifyArcIPConnectivity(ctx context.Context, ipv4, ipv6 bool, a *arc.ARC) error {
	if ipv4 {
		if err := verifyArcIPConnectivityUsingPing(ctx, googleDotComIPv4, a); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyArcIPConnectivityUsingPing(ctx, googleDotComIPv6, a); err != nil {
			return err
		}
	}
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	return nil
}

// RunHostIPSpeedTest runs speedtest on cellular interface and returns the download and upload speeds in bps
func RunHostIPSpeedTest(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, bindir string) (upload, download float64, err error) {
	testing.ContextLog(ctx, "Run IP Connectivity Speed Test")
	var uploadSpeed, downloadSpeed float64 = 0.0, 0.0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := cmd(ctx, filepath.Join(bindir, "speedtest-cli"), "--json").Output()
		if err != nil {
			return errors.Wrap(err, "failed speed test")
		}
		var data map[string]interface{}
		if err := json.Unmarshal(out, &data); err != nil {
			return errors.Wrap(err, "failed to unmarshal output")
		}
		downloadSpeed = data["download"].(float64)
		uploadSpeed = data["upload"].(float64)
		testing.ContextLogf(ctx, " Download %.2f bps", downloadSpeed)
		testing.ContextLogf(ctx, " Upload   %.2f bps", uploadSpeed)
		return nil
	}, &testing.PollOptions{Timeout: speedtestTimeout}); err != nil {
		return 0, 0, errors.Wrap(err, "failed speed test")
	}
	return uploadSpeed, downloadSpeed, nil
}
