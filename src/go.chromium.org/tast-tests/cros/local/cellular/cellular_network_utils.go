// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	pingTimeout      = 60 * time.Second
	curlTimeout      = 60 * time.Second
	defaultInterval  = 300 * time.Millisecond
	speedtestTimeout = 2 * time.Minute
	googleDotComIPv6 = "ipv6.google.com"
	googleDotComIPv4 = "ipv4.google.com"
	testIPv6DotCom   = "ipv6-test.com"
)

func logOutputToFile(ctx context.Context, log, fn string) {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Failed to get out dir")
		return
	}

	outFile, err := os.OpenFile(filepath.Join(outDir, fn), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil || outFile == nil {
		return
	}
	fmt.Fprintf(outFile, "%s\n", time.Now())
	if _, err := outFile.WriteString(log + "\n"); err != nil {
		testing.ContextLogf(ctx, "Failed to write log to %q: %s", fn, err)
		return
	}
	if err := outFile.Close(); err != nil {
		testing.ContextLogf(ctx, "Failed to close %q: %s", fn, err)
	}
}

func verifyCrostiniConnectivityUsingPing(ctx context.Context, binCmd, addr string, cmd func(context.Context, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify connectivity to: ", addr)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if out, err := cmd(ctx, binCmd, "-c1", "-w5", addr).CombinedOutput(); err != nil {
			logOutputToFile(ctx, string(out), "ping-"+addr+".txt")
			return errors.Wrapf(err, "failed to run %s -c1 -w5 %s in Crostini", binCmd, addr)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  pingTimeout,
		Interval: defaultInterval,
	}); err != nil {
		return err
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
	testing.ContextLogf(ctx, "Verify IP%s connectivity using curl to: %s", ipType, addr)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if out, err := cmd(ctx, "curl", "-v", ipType, addr).CombinedOutput(); err != nil {
			testing.ContextLog(ctx, "Failed to run curl : ", err)
			logOutputToFile(ctx, string(out), "curl"+ipType+".txt")
			return errors.Wrapf(err, "failed to run curl -v %s %s", ipType, addr)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  curlTimeout,
		Interval: defaultInterval,
	}); err != nil {
		return err
	}
	return nil
}

// VerifyIPConnectivityUsingCurl verifies the ip connectivity from Host via cellular interface using curl.
func VerifyIPConnectivityUsingCurl(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, bindir string) error {
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	if ipv4 {
		if err := verifyIPConnectivityUsingCurl(ctx, cmd, "", testIPv6DotCom); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyIPConnectivityUsingCurl(ctx, cmd, "-6", testIPv6DotCom); err != nil {
			return err
		}
	}
	return nil
}

func verifyIPConnectivityUsingPing(ctx context.Context, binCmd, iface, addr string, cmd func(context.Context, string, ...string) *testexec.Cmd) error {
	testing.ContextLog(ctx, "Verify IP connectivity to: ", addr)
	if iface != "" {
		testing.ContextLog(ctx, "IP connectivity test bound to interface ", iface)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// ping options as follows:
		//   -v:  verbose
		//   -c1: at most one ICMP packet sent
		//   -w5: stop waiting for ICMP response after 5s
		cmdArgs := []string{"-v", "-c1", "-w5"}
		if iface != "" {
			// -I: bind to the given device
			cmdArgs = append(cmdArgs, "-I", iface)
		}
		cmdArgs = append(cmdArgs, addr)

		if out, err := cmd(ctx, binCmd, cmdArgs...).CombinedOutput(); err != nil {
			logOutputToFile(ctx, string(out), "ping"+addr+iface+".txt")
			return errors.Wrapf(err, "failed to run %s %v", binCmd, cmdArgs)
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

// VerifyIPConnectivityByInterface verifies the ip connectivity from Host via one specific network interface.
// If the |iface| string is empty, the default network interface is used.
func VerifyIPConnectivityByInterface(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, iface, bindir string) error {
	if !ipv4 && !ipv6 {
		return errors.New("no ip network found")
	}
	if ipv4 {
		if err := verifyIPConnectivityUsingPing(ctx, filepath.Join(bindir, "ping"), iface, googleDotComIPv4, cmd); err != nil {
			return err
		}
	}
	if ipv6 {
		if err := verifyIPConnectivityUsingPing(ctx, filepath.Join(bindir, "ping6"), iface, googleDotComIPv6, cmd); err != nil {
			return err
		}
	}
	return nil
}

// VerifyIPConnectivity verifies the ip connectivity from Host via the default network interface
func VerifyIPConnectivity(ctx context.Context, cmd func(context.Context, string, ...string) *testexec.Cmd, ipv4, ipv6 bool, bindir string) error {
	return VerifyIPConnectivityByInterface(ctx, cmd, ipv4, ipv6, "", bindir)
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

// LogL3Info Logs the output of l3info --all to l3info.txt file.
func LogL3Info(ctx context.Context) error {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("failed to get out dir")
	}

	outFile, err := os.Create(filepath.Join(outDir, "l3info.txt"))
	if err != nil {
		return err
	}
	if outFile == nil {
		return errors.New("failed to create l3info.txt file")
	}

	cmd := testexec.CommandContext(ctx, "/usr/local/lib/flimflam/test/l3info", "--all")
	cmd.Stdout = outFile
	cmd.Stderr = outFile

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run l3info")
	}
	if err := outFile.Close(); err != nil {
		return errors.Wrap(err, "failed to close l3info.txt")
	}
	return nil
}
