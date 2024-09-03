// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dhcpd provides the utils to run the ISC DHCP server `dhcpd` inside a
// virtualnet.Env.
// Currently it is only used as DHCPv6-PD server.
package dhcpd

import (
	"bytes"
	"context"
	"html/template"
	"os"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast/core/errors"
)

const confTemplate = `
default-lease-time 2592000;
preferred-lifetime 604800;

option dhcp-renewal-time 3600;
option dhcp-rebinding-time 7200;

subnet6 {{.subnet}} {
       prefix6 {{.delegated_prefix}} {{.delegated_prefix}} /64;
}
`

// Paths in chroot.
const (
	dhcpdPath     = "/usr/local/sbin/dhcpd"
	confPath      = "/tmp/dhcpd.conf"
	leaseFilePath = "/tmp/dhcpd.leases"
	pidPath       = "/tmp/dhcpd.pid"
	tracePath     = "/tmp/dhcpd.trace"
	logPath       = "/tmp/dhcpd.log"
)

type dhcpd struct {
	env *env.Env

	subnet *subnet.IPv6Subnet

	cmd *testexec.Cmd
}

// Option is a type of function to configure dhcpd.
type Option = func(*dhcpd)

// WithDHCPPD enables DHCPv6-PD server function in dhcpd. subnet specifies the
// DHCP subnet, and must be of at least /63 size. The second address in subnet
// will be used as the gateway address, and the second /64 in the subnet will be
// delegated to IA_PD clients.
func WithDHCPPD(subnet *subnet.IPv6Subnet) Option {
	return func(d *dhcpd) {
		d.subnet = subnet
	}
}

// New creates a new dhcpd object. The returned object can be passed to
// Env.StartServer(), its lifetime will be managed by the Env object.
func New(opts ...Option) *dhcpd {
	d := &dhcpd{}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Start starts the dhcpd process.
func (d *dhcpd) Start(ctx context.Context, env *env.Env) error {
	d.env = env

	// Prepare config file.
	confVals := map[string]interface{}{}

	if d.subnet != nil {
		serverAddr := d.subnet.GetAddrEndWith(2)
		delegatedPrefix := d.subnet.GetSecondSlash64()

		confVals["subnet"] = d.subnet.String()
		confVals["delegated_prefix"] = delegatedPrefix.IP.String()

		// Install gateway address and routes.
		if err := d.env.ConfigureInterface(ctx, d.env.VethInName, serverAddr, d.subnet); err != nil {
			return errors.Wrap(err, "failed to configure IP/route in router netns")
		}
	}

	b := &bytes.Buffer{}
	template.Must(template.New("").Parse(confTemplate)).Execute(b, confVals)
	if err := os.WriteFile(d.env.ChrootPath(confPath), []byte(b.String()), 0644); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}

	if err := os.WriteFile(d.env.ChrootPath(leaseFilePath), []byte(""), 0644); err != nil {
		return errors.Wrap(err, "failed to create leases file")
	}

	// Start the command.
	cmd := []string{
		dhcpdPath,
		"-6",
		"-d",
		"-cf", confPath,
		"-lf", leaseFilePath,
		"-pf", pidPath,
		"-tf", tracePath,
	}
	d.cmd = d.env.CreateCommand(ctx, cmd...)

	logFd, err := os.Create(d.env.ChrootPath(logPath))
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer logFd.Close()
	d.cmd.Stdout = logFd
	d.cmd.Stderr = logFd
	if err := d.cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start dhcpd daemon")
	}

	return nil
}

// Stop stops the dhcpd process.
func (d *dhcpd) Stop(ctx context.Context) error {
	if d.cmd == nil || d.cmd.Process == nil {
		return nil
	}
	if err := d.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to kill dhcpd processs")
	}
	d.cmd.Wait()
	d.cmd = nil
	return nil
}

// WriteLogs writes traces into |f|.
func (d *dhcpd) WriteLogs(ctx context.Context, f *os.File) error {
	return d.env.ReadAndWriteLogIfExists(d.env.ChrootPath(logPath), f)
}
