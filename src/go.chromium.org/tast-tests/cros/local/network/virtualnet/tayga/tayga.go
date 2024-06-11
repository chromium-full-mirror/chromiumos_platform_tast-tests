// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tayga provides the utils to run the tayga daemon inside a
// virtualnet.Env, together with kernel built-in NAT66, to setup a simple NAT64
// environment for the CLAT testing.
package tayga

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Paths in chroot.
const (
	tunName         = "nat64"
	wellKnownPrefix = "64:ff9b::/96"
	taygaPath       = "/usr/sbin/tayga"
	confPath        = "/tmp/tayga.conf"
)

type addrMap struct {
	ipv4 string
	ipv6 string
}

type tayga struct {
	env *env.Env
	cmd *testexec.Cmd

	ipv4TaygaAddr  string
	ipv6TaygaAddr  string
	ipv4MappedAddr string
	ipv6MappedAddr string
	prefix         string
}

// New creates a new tayga object. The returned object can be passed to
// Env.StartServer(), its lifetime will be managed by the Env object. This
// function needs two subnet for the address translation for non-public
// addresses.
func New(ipv4Subnet *subnet.IPv4Subnet, ipv6Subnet *subnet.IPv6Subnet) *tayga {
	t := &tayga{
		// TaygaAddrs will be installed on the tun interface.
		ipv4TaygaAddr: ipv4Subnet.GetAddrEndWith(1).String(),
		ipv6TaygaAddr: ipv6Subnet.GetAddrEndWith(1).String(),
		// MappedAddrs will be used to be translated to each other. IPv6 packets
		// sent to tayga will be SNAT-ed to the mapped IPv6 addr, and sent out again
		// by tayga with the mapped IPv4 addr.
		ipv4MappedAddr: ipv4Subnet.GetAddrEndWith(2).String(),
		ipv6MappedAddr: ipv6Subnet.GetAddrEndWith(2).String(),
		prefix:         wellKnownPrefix,
	}
	return t
}

// Start starts the tayga process.
func (t *tayga) Start(ctx context.Context, env *env.Env) (retErr error) {
	t.env = env

	var lines []string
	lines = append(lines, "tun-device "+tunName)
	lines = append(lines, "ipv4-addr "+t.ipv4TaygaAddr)
	lines = append(lines, "ipv6-addr "+t.ipv6TaygaAddr)
	lines = append(lines, "prefix "+t.prefix)
	lines = append(lines, "map "+t.ipv4MappedAddr+" "+t.ipv6MappedAddr)
	if err := os.WriteFile(t.env.ChrootPath(confPath), []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}

	// Start the command.
	cmd := []string{
		taygaPath,
		"-n", // Do not detach.
		"-c", confPath,
	}
	t.cmd = t.env.CreateCommand(ctx, cmd...)
	if err := t.cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start tayga process")
	}
	defer func() {
		if retErr != nil {
			if err := t.Stop(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to clean up tayga after start failure: ", err)
			}
		}
	}()

	if err := testing.Poll(ctx, func(c context.Context) error {
		return t.env.RunWithoutChroot(ctx, "ip", "link", "show", "dev", tunName)
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for tun interface ready")
	}

	// Set up datapath.
	cmds := []string{
		"ip link set up dev " + tunName,
		fmt.Sprintf("ip -4 addr add %s/32 dev %s", t.ipv4TaygaAddr, tunName),
		fmt.Sprintf("ip -6 addr add %s/128 dev %s", t.ipv6TaygaAddr, tunName),
		fmt.Sprintf("ip -4 route add %s dev %s", t.ipv4MappedAddr, tunName),
		fmt.Sprintf("ip -6 route add %s dev %s", t.ipv6MappedAddr, tunName),
		fmt.Sprintf("ip -6 route add %s dev %s", t.prefix, tunName),
		fmt.Sprintf("ip6tables -t nat -A POSTROUTING -o %s -j SNAT --to-source %s -w", tunName, t.ipv6MappedAddr),
	}
	for _, cmdStr := range cmds {
		if err := t.env.RunWithoutChroot(ctx, strings.Split(cmdStr, " ")...); err != nil {
			return errors.Wrapf(err, "failed to run %s to setup tun interface", cmdStr)
		}
	}

	return nil
}

// Stop stops the tayga process.
func (t *tayga) Stop(ctx context.Context) error {
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	if err := t.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to kill tayga process")
	}
	t.cmd.Wait()
	t.cmd = nil
	return nil
}

// WriteLogs does nothing here. tayga will write logs to messages.
func (t *tayga) WriteLogs(ctx context.Context, f *os.File) error {
	return nil
}
