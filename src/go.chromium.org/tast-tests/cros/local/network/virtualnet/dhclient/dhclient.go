// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dhclient provides the utils to run the dhclient daemon inside a
// virtualnet.Env. This simulates an IPv4 DHCP client.
package dhclient

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Paths in chroot.
const (
	cmdPath    = "/usr/local/sbin/dhclient"
	scriptPath = "/usr/local/sbin/dhclient-script"
	confPath   = "/tmp/dhclient.conf"
	leasePath  = "/tmp/dhclient.leases"
	pidPath    = "/tmp/dhclient.pid"
)

type dhclient struct {
	iface    string
	hostname string
	env      *env.Env
	cmd      *testexec.Cmd
}

// New creates a new dhclient object. The returned object can be passed to
// Env.StartServer(), its lifetime will be managed by the Env object.
func New(iface, hostname string) *dhclient {
	return &dhclient{iface: iface, hostname: hostname}
}

// Start starts the dhclient process, and wait until the dhclient set the IP on
// the interface.
func (d *dhclient) Start(ctx context.Context, env *env.Env) error {
	d.env = env

	conf := fmt.Sprintf(`interface "%s" { send host-name "%s"; }`, d.iface, d.hostname)
	if err := os.WriteFile(d.env.ChrootPath(confPath), []byte(conf), 0644); err != nil {
		return errors.Wrap(err, "failed to write dhclient config into file")
	}

	// Start the command.
	cmd := []string{
		cmdPath,
		"-d", // Run as a foreground process
		"-cf", d.env.ChrootPath(confPath),
		"-lf", d.env.ChrootPath(leasePath),
		"-pf", d.env.ChrootPath(pidPath),
		"-sf", scriptPath,
		d.iface,
	}
	d.cmd = d.env.CreateCommandWithoutChroot(ctx, cmd...)
	if err := d.cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start the dhclient daemon")
	}

	// Wait for DHCP IP is set.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cmd := fmt.Sprintf(`ip -j -4 addr show %s | jq .[0].addr_info[0].local`, d.iface)
		output, err := d.env.CreateCommandWithoutChroot(ctx, "/bin/sh", "-c", cmd).Output()
		if err != nil {
			return testing.PollBreak(err)
		}

		ip := strings.TrimSpace(string(output))
		if ip != "null" {
			testing.ContextLog(ctx, "DHCP IP is set: ", ip)
			return nil
		}
		return errors.New("dhclient hasn't set IP on " + d.iface)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "dhclient failed to set IP on "+d.iface)
	}

	return nil
}

// Stop stops the dhclient process.
func (d *dhclient) Stop(ctx context.Context) error {
	if d.cmd == nil || d.cmd.Process == nil {
		return nil
	}
	if err := d.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to kill dhclient processs")
	}
	d.cmd.Wait()
	d.cmd = nil
	return nil
}

// WriteLogs writes logs into |f|.
func (d *dhclient) WriteLogs(ctx context.Context, f *os.File) error {
	// dhclient writes the log to /var/log/messages, so we don't have way to
	// write log into |f|.
	return nil
}
