// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wpacli

import (
	"context"
	"strconv"

	"go.chromium.org/tast/core/testing"
)

// setP2PGOAddConf contains the optional information for "p2p_group_add" function.
type setP2PGroupAddConf struct {
	freq int
	mode PhyMode
}

// P2PGOOption is a function signature that modifies P2PGroupAdd.
type P2PGOOption func(*setP2PGroupAddConf)

// SetP2PGOFreq returns a P2PGroupAddOption which sets the first center frequency (in MHz).
func SetP2PGOFreq(f int) P2PGOOption {
	return func(c *setP2PGroupAddConf) {
		c.freq = f
	}
}

// SetP2PGOMode returns a P2PGroupAddOption which sets the mode.
func SetP2PGOMode(m PhyMode) P2PGOOption {
	return func(c *setP2PGroupAddConf) {
		c.mode = m
	}
}

// P2PGroupAdd add a new P2P group (local end as GO).
func (r *Runner) P2PGroupAdd(ctx context.Context, ops ...P2PGOOption) error {
	conf := &setP2PGroupAddConf{
		freq: 2462,        // Default 2462 MHz.
		mode: PhyModeHT40, // Default ht40.
	}
	for _, op := range ops {
		op(conf)
	}
	return r.run(ctx, "OK", "p2p_group_add", "freq="+strconv.Itoa(conf.freq), string(conf.mode))
}

// P2PGroupAddPersistent connects to a P2P GO device.
func (r *Runner) P2PGroupAddPersistent(ctx context.Context) error {
	// persistent=0: Specify a restart of a persistent group (connect to an existing persistent group).
	return r.run(ctx, "OK", "p2p_group_add", "persistent=0")
}

// P2PGroupRemove removes P2P group interface (local end as GO).
func (r *Runner) P2PGroupRemove(ctx context.Context, iface string) error {
	return r.run(ctx, "OK", "p2p_group_remove", iface)
}

// P2PFlush flush P2P state.
func (r *Runner) P2PFlush(ctx context.Context) error {
	return r.run(ctx, "OK", "p2p_flush")
}

// P2PAddGONetwork adds the GO network in the client device.
func (r *Runner) P2PAddGONetwork(ctx context.Context, ssid, passphrase string) (int, error) {
	successfulRun := false
	networkID, err := r.addNetwork(ctx)
	if err != nil {
		return -1, err
	}
	defer func(ctx context.Context) {
		if !successfulRun {
			if err := r.RemoveNetwork(ctx, networkID); err != nil {
				testing.ContextLog(ctx, "Failed to remove the network: ", err)
			}
		}
	}(ctx)
	if err := r.setNetwork(ctx, networkID, "ssid", strconv.Quote(ssid)); err != nil {
		return -1, err
	}
	if err := r.setNetwork(ctx, networkID, "psk", strconv.Quote(passphrase)); err != nil {
		return -1, err
	}
	// disabled=2: Indicate special network block use as a P2P persistent group information.
	if err := r.setNetwork(ctx, networkID, "disabled", "2"); err != nil {
		return -1, err
	}
	successfulRun = true

	return networkID, nil
}
