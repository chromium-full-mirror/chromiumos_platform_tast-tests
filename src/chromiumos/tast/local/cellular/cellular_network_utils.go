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
	"chromiumos/tast/testing"
)

const (
	pingTimeout      = 10 * time.Second
	curlTimeout      = 30 * time.Second
	speedtestTimeout = 2 * time.Minute
	googleDotComIPv6 = "ipv6.google.com"
	googleDotComIPv4 = "ipv4.google.com"
)

func verifyCrostiniIPv4ConnectivityUsingPing(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify IPv4 connectivity")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "/bin/ping", "-c1", "-w1", googleDotComIPv4).Run(); err != nil {
			return errors.Wrap(err, "failed ipv4 ping test in Crostini")
		}
		return nil
	}, &testing.PollOptions{Timeout: pingTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv4 ping test in Crostini")
	}
	return nil
}

func verifyCrostiniIPv6ConnectivityUsingPing(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify IPv6 connectivity")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "/bin/ping6", "-c1", "-w1", googleDotComIPv6).Run(); err != nil {
			return errors.Wrap(err, "failed ipv6 ping test in Crostini")
		}
		return nil
	}, &testing.PollOptions{Timeout: pingTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv6 ping test in Crostini")
	}
	return nil
}

func verifyCrostiniIPv4ConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify IPv4 connectivity")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "curl", googleDotComIPv4).Run(); err != nil {
			return errors.Wrap(err, "failed ipv4 curl test in Crostini")
		}
		return nil
	}, &testing.PollOptions{Timeout: curlTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv4 curl test in Crostini")
	}
	return nil
}

func verifyCrostiniIPv6ConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify IPv6 connectivity")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "curl", "-6", googleDotComIPv6).Run(); err != nil {
			return errors.Wrap(err, "failed ipv6 curl test in Crostini")
		}
		return nil
	}, &testing.PollOptions{Timeout: curlTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv6 curl test in Crostini")
	}
	return nil
}

// VerifyCrostiniIPConnectivityUsingCurl verifies the ip connectivity from crostini via cellular interface using curl.
func VerifyCrostiniIPConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd, ipv4, ipv6 bool) error {
	if ipv4 {
		if err := verifyCrostiniIPv4ConnectivityUsingCurl(ctx, cmd); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyCrostiniIPv6ConnectivityUsingCurl(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

// VerifyCrostiniIPConnectivity verifies the ip connectivity from crostini via cellular interface using ping.
func VerifyCrostiniIPConnectivity(ctx context.Context, cmd func(context.Context, ...string) *testexec.Cmd, ipv4, ipv6 bool) error {
	if ipv4 {
		if err := verifyCrostiniIPv4ConnectivityUsingPing(ctx, cmd); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyCrostiniIPv6ConnectivityUsingPing(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

func verifyIPv4ConnectivityUsingPing(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, bindir string) error {
	testing.ContextLog(ctx, "Verify IPv4 connectivity")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, filepath.Join(bindir, "ping"), "-c1", "-w1", googleDotComIPv4).Run(); err != nil {
			return errors.Wrap(err, "failed ipv4 ping test")
		}
		return nil
	}, &testing.PollOptions{Timeout: pingTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv4 ping test")
	}
	return nil
}

func verifyIPv6ConnectivityUsingPing(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, bindir string) error {
	testing.ContextLog(ctx, "Verify IPv6 connectivity")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, filepath.Join(bindir, "ping6"), "-c1", "-w1", googleDotComIPv6).Run(); err != nil {
			return errors.Wrap(err, "failed ipv6 ping test")
		}
		return nil
	}, &testing.PollOptions{Timeout: pingTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv6 ping test")
	}
	return nil
}

func verifyIPv4ConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, bindir string) error {
	testing.ContextLog(ctx, "Verify IPv4 connectivity using curl")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "curl", googleDotComIPv4).Run(); err != nil {
			return errors.Wrap(err, "failed ipv4 curl test")
		}
		return nil
	}, &testing.PollOptions{Timeout: curlTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv4 curl test")
	}
	return nil
}

func verifyIPv6ConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, bindir string) error {
	testing.ContextLog(ctx, "Verify IPv6 connectivity using curl")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cmd(ctx, "curl", "-6", googleDotComIPv6).Run(); err != nil {
			return errors.Wrap(err, "failed ipv6 curl test")
		}
		return nil
	}, &testing.PollOptions{Timeout: curlTimeout}); err != nil {
		return errors.Wrap(err, "failed ipv6 curl test")
	}
	return nil
}

// VerifyIPConnectivityUsingCurl verifies the ip connectivity from Host via cellular interface using curl.
func VerifyIPConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, bindir string) error {
	if ipv4 {
		if err := verifyIPv4ConnectivityUsingCurl(ctx, cmd, bindir); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyIPv6ConnectivityUsingCurl(ctx, cmd, bindir); err != nil {
			return err
		}
	}
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	return nil
}

// VerifyIPConnectivity verifies the ip connectivity from ARC via cellular interface using ping.
func VerifyIPConnectivity(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, bindir string) error {
	if ipv4 {
		if err := verifyIPv4ConnectivityUsingPing(ctx, cmd, bindir); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyIPv6ConnectivityUsingPing(ctx, cmd, bindir); err != nil {
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
