// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shill

import (
	"context"
	"net"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

type cidr struct {
	IP     net.IP
	Subnet *net.IPNet
}

// NetworkConfig represents the NetworkConfig dict in Server properties. See the
// NetworkConfig property in platform2/shill/doc/services-api.txt for details.
type NetworkConfig struct {
	IPv4CIDR      cidr
	IPv4Gateway   net.IP
	IPv6Addresses []cidr
	IPv6Gateway   net.IP
	NameServers   []net.IP
	SearchDomains []string
	MTU           int32
}

func parseNetworkConfigProperty(ctx context.Context, dict map[string]interface{}) (*NetworkConfig, error) {
	ret := &NetworkConfig{}

	// Empty dict is valid.
	if len(dict) == 0 {
		return ret, nil
	}

	props := dbusutil.NewProperties(dict)

	var errs []error

	// A group of helper functions which return a single value directly on
	// success, or append error to errs on failure.
	getString := func(key string) string {
		val, err := props.GetString(key)
		if err != nil {
			errs = append(errs, err)
			return ""
		}
		return val
	}
	getStrings := func(key string) []string {
		val, err := props.GetStrings(key)
		if err != nil {
			errs = append(errs, err)
			return []string{}
		}
		return val
	}
	getInt32 := func(key string) int32 {
		val, err := props.GetInt32(key)
		if err != nil {
			errs = append(errs, err)
			return 0
		}
		return val
	}
	parseCIDR := func(val string) cidr {
		ip, subnet, err := net.ParseCIDR(val)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to parse %s as cidr", val))
			return cidr{}
		}
		return cidr{ip, subnet}
	}

	ret.IPv4CIDR = parseCIDR(getString("IPv4Address"))
	ret.IPv4Gateway = net.ParseIP(getString("IPv4Gateway"))
	for _, val := range getStrings("IPv6Addresses") {
		ret.IPv6Addresses = append(ret.IPv6Addresses, parseCIDR(val))
	}
	ret.IPv6Gateway = net.ParseIP(getString("IPv6Gateway"))
	for _, val := range getStrings("NameServers") {
		ret.NameServers = append(ret.NameServers, net.ParseIP(val))
	}
	ret.SearchDomains = getStrings("SearchDomains")
	ret.MTU = getInt32("MTU")

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return ret, nil
}

// HasIPv4Address returns whether this config has a valid IPv4 address.
func (c *NetworkConfig) HasIPv4Address() bool {
	return c.IPv4CIDR.IP != nil && c.IPv4CIDR.IP.To4() != nil && !c.IPv4CIDR.IP.IsUnspecified()
}

// HasIPv6Address returns whether this config has a valid IPv6 address.
func (c *NetworkConfig) HasIPv6Address() bool {
	return len(c.IPv6Addresses) != 0 && !c.IPv4CIDR.IP.IsUnspecified()
}

// IPv4NameServers returns IPv4 name servers.
func (c *NetworkConfig) IPv4NameServers() []net.IP {
	var ret []net.IP
	for _, ip := range c.NameServers {
		if ip.To4() != nil && !ip.IsUnspecified() {
			ret = append(ret, ip)
		}
	}
	return ret
}

// IPv6NameServers returns IPv6 name servers.
func (c *NetworkConfig) IPv6NameServers() []net.IP {
	var ret []net.IP
	for _, ip := range c.NameServers {
		if ip.To4() == nil && len(ip) == net.IPv6len && !ip.IsUnspecified() {
			ret = append(ret, ip)
		}
	}
	return ret
}
