// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"net"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	remoteip "go.chromium.org/tast-tests/cros/remote/network/ip"
	remoteping "go.chromium.org/tast-tests/cros/remote/network/ping"
	"go.chromium.org/tast-tests/cros/remote/wificell/router"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// Implementation of WiFiDevice interface for Router.

// RouterData contains all information necessary to configure router.
type RouterData struct {
	target string
	host   *ssh.Conn
	object router.Base
	br     *bridgeData
	brveth *bridgeVethData
}

// Type returns RouterDevice type.
func (rd *RouterData) Type() WiFiDeviceType {
	return RouterDevice
}

// Conn returns pointer to SSH connection object.
func (rd *RouterData) Conn() *ssh.Conn {
	return rd.host
}

// findManagedInterface looks for the first managed interface that happens to be used (== has IPv4 configured.)
func (rd *RouterData) findManagedInterface(ctx context.Context) (string, []net.IP, error) {
	ipr := remoteip.NewRemoteRunner(rd.Conn())

	names, err := ipr.LinkWithPrefix(ctx, "managed")
	if err != nil {
		return "", nil, err
	}
	for _, name := range names {
		ips, err := ipr.IPAddr(ctx, name)
		if err != nil {
			// Interface may be down.
			testing.ContextLogf(ctx, "Interface %q does not contain valid IPv4 address, reason: %v", name, err)
			continue
		}
		var ret []net.IP
		for _, ip := range ips {
			if ip.To4() != nil {
				ret = append(ret, ip)
			}
		}
		if len(ret) != 0 {
			return name, ret, nil
		}
	}
	return "", nil, errors.Errorf("Interfaces: %+v don't contain any active managed interface", names)
}

// IfName returns interface name of a particular type (STA/AP/P2P)
func (rd *RouterData) IfName(ctx context.Context, ifType IfaceType) (string, error) {
	switch ifType {
	case APIfaceType:
		ifName, _, err := rd.findManagedInterface(ctx)
		return ifName, err

	default:
		return "", errors.Errorf("Interface type %v not implemented", ifType)
	}
}

// IPv4Addrs returns a slice of IPv4 Addresses configured on an interface of a particular type.
func (rd *RouterData) IPv4Addrs(ctx context.Context, ifType IfaceType) ([]net.IP, error) {
	switch ifType {
	case APIfaceType:
		_, ips, err := rd.findManagedInterface(ctx)
		return ips, err

	default:
		return nil, errors.Errorf("Interface type %v not implemented", ifType)
	}
}

// PingDevice pings another WiFiDevice on a particular type of interface. It automatically detects necessary addresses.
func (rd *RouterData) PingDevice(ctx context.Context, dest WiFiDevice,
	srcIfType, dstIfType IfaceType, opts ...ping.Option) (*ping.Result, error) {
	srcName, err := rd.IfName(ctx, srcIfType)
	if err != nil {
		return nil, err
	}

	ip, err := dest.IPv4Addrs(ctx, dstIfType)
	if err != nil {
		return nil, err
	}
	opts = append(opts, ping.Interval(0.1), ping.BindAddress(true), ping.SourceIface(srcName))

	testing.ContextLogf(ctx, "Ping %s:%s -> %s", rd.object.RouterName(), srcName, ip[0])
	return rd.Ping(ctx, ip[0].String(), dstIfType, opts...)
}

// Ping sends pings from the current device to an abstract IP Address.
func (rd *RouterData) Ping(ctx context.Context, addr string, ifType IfaceType, opts ...ping.Option) (*ping.Result, error) {
	pr := remoteping.NewRemoteRunner(rd.Conn())
	return pr.Ping(ctx, addr, opts...)
}

// Other definitions

// RouterType returns encapsulated router type.
func (rd *RouterData) RouterType() support.RouterType {
	return rd.object.RouterType()
}
