// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package p2p contains p2p-related utility functions.
package p2p

const (
	p2pDefaultFreq int = 2462
)

// setP2PGOAddConf contains the optional information for "p2pGroupAdd" function.
type setP2PGroupAddConf struct {
	freq      int
	networkID int
}

// GroupOption is a function signature that modifies P2PGroupAdd.
type GroupOption func(*setP2PGroupAddConf)

// SetFreq returns a P2PGroupAddOption which sets the first center frequency (in MHz).
func SetFreq(f int) GroupOption {
	return func(c *setP2PGroupAddConf) {
		c.freq = f
	}
}

// SetNetworkID returns a P2PGroupAddOption which sets the NetworkID of the group.
func SetNetworkID(id int) GroupOption {
	return func(c *setP2PGroupAddConf) {
		c.networkID = id
	}
}

// Freq returns frequency encoded in the set of P2PGOOption elements.
func Freq(ops ...GroupOption) int {
	conf := &setP2PGroupAddConf{}
	for _, op := range ops {
		op(conf)
	}
	return conf.freq
}

// NetworkID returns network ID encoded in the set of P2PGOOption elements.
func NetworkID(ops ...GroupOption) int {
	conf := &setP2PGroupAddConf{
		networkID: -1,
	}
	for _, op := range ops {
		op(conf)
	}
	return conf.networkID
}
