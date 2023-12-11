// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package subnet contains the utils to create a subnet pool which can be used
// to allocate private subnet to be used in a virtualnet.
package subnet

import (
	"net"

	"go.chromium.org/tast/core/errors"
)

// Pool allocates available subnets for IPv4 and IPv6.
type Pool struct {
	// ipv4Next is the next available subnet id for IPv4.
	ipv4Next int
	// ipv6Next is the next available subnet id for IPv6.
	ipv6Next int
}

// IPSubnet defines the functions on IPv4Subnet and IPv6Subnet.
type IPSubnet interface {
	// String returns the CIDR-notation of the prefix, e.g., "192.168.1.0/24".
	String() string
}

// IPv4Subnet represents an IPv4 subnet. This is a wrapper of the net.IPNet
// struct and provides helpful utils in the test.
type IPv4Subnet struct {
	net.IPNet
}

// IPv6Subnet represents an IPv6 subnet. This is a wrapper of the net.IPNet
// struct and provides helpful utils in the test.
type IPv6Subnet struct {
	net.IPNet
}

// The range of available subnet ids. Each pool can allocate 100 subnets for
// IPv4 and IPv6 separately, which should be enough for most of the tests.
const (
	subnetStart = 100
	subnetEnd   = 200
)

// NewPool creates a new pool for IP subnets. The same pool should be used
// in one test to avoid address collisions.
func NewPool() *Pool {
	return &Pool{ipv4Next: subnetStart, ipv6Next: subnetStart}
}

// AllocNextIPv4Subnet allocates the next IPv4 subnet.
func (p *Pool) AllocNextIPv4Subnet() (*IPv4Subnet, error) {
	if p.ipv4Next > subnetEnd {
		return nil, errors.New("no available subnet")
	}
	id := p.ipv4Next
	p.ipv4Next++
	return &IPv4Subnet{
		IPNet: net.IPNet{
			IP:   net.IPv4(192, 168, byte(id), 0),
			Mask: net.IPv4Mask(255, 255, 255, 0),
		},
	}, nil
}

// AllocNextIPv6Subnet allocates the next IPv6 subnet.
func (p *Pool) AllocNextIPv6Subnet() (*IPv6Subnet, error) {
	if p.ipv4Next > subnetEnd {
		return nil, errors.New("no available subnet")
	}
	id := p.ipv6Next
	p.ipv6Next++
	return &IPv6Subnet{
		IPNet: net.IPNet{
			IP:   []byte{0xfd, 0, 0, 0, 0, 0, 0, byte(id), 0, 0, 0, 0, 0, 0, 0, 0},
			Mask: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0},
		},
	}, nil
}
