// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"math/rand"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type executableCmd string

const (
	// Executables path of iptables.
	iptablesCmd  executableCmd = "iptables"
	ip6tablesCmd executableCmd = "ip6tables"
)

// ExecFuncOnChromeOffline disconnect Chrome browser internet connection through iptables.
// Then it executes given function and reverts it back to the original state.
func ExecFuncOnChromeOffline(ctx context.Context, f func(ctx context.Context) error) (result error) {
	// Reserve 5 seconds to resume firewall settings.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Block Chrome traffic in iptables.
	cleanupIP, err := blockChromeTraffic(ctx, iptablesCmd)
	if err != nil {
		return errors.Wrapf(err, "failed to block chrome traffic in %s", iptablesCmd)
	}
	defer func(cleanupCtx context.Context) {
		if err := cleanupIP(cleanupCtx); err != nil {
			result = errors.Wrapf(err, "failed to resume %s. Offline test result: %v", iptablesCmd, result)
		} else {
			testing.ContextLogf(ctx, "Resumed %s", iptablesCmd)
		}
	}(cleanupCtx)

	// Block Chrome traffic in ip6tables.
	cleanupIP6, err := blockChromeTraffic(ctx, ip6tablesCmd)
	if err != nil {
		return errors.Wrapf(err, "failed to block chrome traffic in %s", ip6tablesCmd)
	}
	defer func(cleanupCtx context.Context) {
		if err := cleanupIP6(cleanupCtx); err != nil {
			result = errors.Wrapf(err, "failed to resume %s. Offline test result: %v", ip6tablesCmd, result)
		} else {
			testing.ContextLogf(ctx, "Resumed %s", ip6tablesCmd)
		}
	}(cleanupCtx)

	// Run the real test function here.
	result = f(ctx)
	return
}

// blockChromeTraffic blocks iptables / ip6tables and returns cleanup function.
func blockChromeTraffic(ctx context.Context, ipcmd executableCmd) (func(cleanupCtx context.Context) error, error) {
	// Block all output traffic from chronos user, except localhost.
	// Localhost must be excluded from this rule, otherwise tast server cannot be connected either.
	blockArgs := []string{"OUTPUT", "-m", "owner", "--uid-owner", "chronos", "!", "-o", "lo", "-j", "REJECT", "-w"}

	// Insert blocking rule into the first line
	blockChromeCommand := testexec.CommandContext(ctx, string(ipcmd), append([]string{"-t", "filter", "-I"}, blockArgs...)...)

	// Block Chrome traffic in iptables / ip6tables.
	testing.ContextLogf(ctx, "Blocking Chrome internet connection in: %s", string(ipcmd))
	if err := blockChromeCommand.Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrapf(err, "failed to block chrome traffic in %s with command %v", string(ipcmd), blockArgs)
	}

	return func(cleanupCtx context.Context) error {
		resumeChromeCommand := testexec.CommandContext(cleanupCtx, string(ipcmd), append([]string{"-t", "filter", "-D"}, blockArgs...)...)
		return resumeChromeCommand.Run(testexec.DumpLogOnError)
	}, nil
}

// UnusedOrRandomPort returns an unused port if possible, or a random one if not.
func UnusedOrRandomPort(ctx context.Context, fam l4server.Family) int {
	rport := func(err error) int {
		testing.ContextLog(ctx, "Failed to query unused port - generating one randomly, good luck: ", err)
		return 10000 + rand.Intn(50000)
	}

	addr, err := net.ResolveUDPAddr(fam.String(), "localhost:0")
	if err != nil {
		return rport(err)
	}
	c, err := net.ListenUDP(fam.String(), addr)
	if err != nil {
		return rport(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}
