// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package addrutil provides general utility function regarding network
// addresses.
package addrutil

import (
	"net"

	"go.chromium.org/tast/core/errors"
)

// IfaceAddrs represents the IP addresses configured on an interface.
type IfaceAddrs struct {
	// IPv4Addr is the IPv4 address on the interface. There is only one IPv4
	// address on an interface.
	IPv4Addr net.IP
	// IPv6Addrs is the list of IPv6 addresses (excluding link-local address) on
	// the interface.
	IPv6Addrs []net.IP
	// LinkLocalIPv6Addr is the IPv6 link local address on the interface.
	LinkLocalIPv6Addr net.IP
}

// All returns all addresses (excluding the IPv6 link-local address) on this
// interface.
func (addrs *IfaceAddrs) All() []net.IP {
	var ret []net.IP
	if addrs.IPv4Addr != nil {
		ret = append(ret, addrs.IPv4Addr)
	}
	return append(ret, addrs.IPv6Addrs...)
}

// ReadInterfaceAddresses reads the current IP addresses configured on a network
// interface from kernel, and returns them classified to IPv4, IPv6 global, and
// IPv6 link local ones. It also converts the values from net.Addr into an
// easier-to-use net.IP format before returning.
func ReadInterfaceAddresses(ifname string) (retAddrs *IfaceAddrs, retErr error) {
	var ret IfaceAddrs
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get interface object for interface %s", ifname)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list addrs on interface %s", ifname)
	}

	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse CIDR string %s", addr)
		}
		if ipv4Addr := ip.To4(); ipv4Addr != nil {
			if ret.IPv4Addr != nil {
				return nil, errors.Errorf("there are two IPv4 addrs %s and %s on interface %s", ret.IPv4Addr, ipv4Addr, ifname)
			}
			ret.IPv4Addr = ipv4Addr
			continue
		}
		if ipv6Addr := ip.To16(); ipv6Addr != nil {
			if ipv6Addr.IsLinkLocalUnicast() {
				if ret.LinkLocalIPv6Addr != nil {
					return nil, errors.Errorf("there are two link local IPv6 addrs %s and %s on interface %s", ret.LinkLocalIPv6Addr, ipv6Addr, ifname)
				}
				ret.LinkLocalIPv6Addr = ipv6Addr
				continue
			}
			ret.IPv6Addrs = append(ret.IPv6Addrs, ipv6Addr)
			continue
		}
		return nil, errors.Wrapf(err, "%s is neither a v4 addr nor a v6 addr", ip)
	}
	return &ret, nil
}
