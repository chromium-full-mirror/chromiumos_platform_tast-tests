// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wpacli

import (
	"context"
	"net"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// p2pGroupAdd add a new P2P group. It does either of two things:
// If networkID is not present in ops, it creates new P2P group and becomes GO.
// If networkID is present in ops, it connects to the existing P2P group.
func (r *Runner) p2pGroupAdd(ctx context.Context, ops ...p2p.GroupOption) error {
	cmd := []string{"p2p_group_add"}
	// The "persistent" parameter works counterintuitively: if it's omitted,
	// the group is created as GO, if it's present the group is created in the Client mode.
	if id := p2p.NetworkID(ops...); id != -1 {
		cmd = append(cmd, "persistent="+strconv.Itoa(id))
	}
	if freq := p2p.Freq(ops...); freq > 0 {
		cmd = append(cmd, "freq="+strconv.Itoa(freq))
	}

	return r.run(ctx, "OK", cmd...)
}

// p2pGroupRemove removes P2P group interface (local end as GO).
func (r *Runner) p2pGroupRemove(ctx context.Context, iface string) error {
	return r.run(ctx, "OK", "p2p_group_remove", iface)
}

// p2pFlush flush P2P state.
func (r *Runner) p2pFlush(ctx context.Context) error {
	return r.run(ctx, "OK", "p2p_flush")
}

// p2pAddNetwork adds the GO network in the client device.
func (r *Runner) p2pAddNetwork(ctx context.Context, ssid, passphrase string) (int, error) {
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
	if err := r.setNetwork(ctx, networkID, "ssid", strconv.Quote(string(ssid))); err != nil {
		return -1, err
	}
	if err := r.setNetwork(ctx, networkID, "psk", strconv.Quote(string(passphrase))); err != nil {
		return -1, err
	}
	// disabled=2: Indicate special network block use as a P2P persistent group information.
	if err := r.setNetwork(ctx, networkID, "disabled", "2"); err != nil {
		return -1, err
	}
	successfulRun = true

	return networkID, nil
}

// p2pGroupStartedWait waits for the group and returns group configuration.
func (r *Runner) p2pGroupStartedWait(ctx context.Context, wpaMonitor *WPAMonitor) (
	p2pGOIface, p2pGroupSSID, p2pGroupPassphrase string, _ error) {
	const waitForP2PGroupStartedTimeout = 30 * time.Second
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		event, err := wpaMonitor.WaitForEvent(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to wait for P2PGroupStartedEvent"))
		}
		if event == nil { // timeout
			return testing.PollBreak(errors.New("timed out waiting for P2PGroupStartedEvent"))
		}
		if evt, ok := event.(*P2PGroupStartedEvent); ok {
			testing.ContextLogf(ctx, "New event: {%v} received", evt)
			p2pGOIface = evt.IfaceName
			p2pGroupSSID = evt.SSID
			p2pGroupPassphrase = evt.Passphrase
			return nil
		}

		return errors.New("no P2PGroupStartedEvent found")
	}, &testing.PollOptions{Timeout: waitForP2PGroupStartedTimeout}); err != nil {
		return "", "", "", err
	}
	return p2pGOIface, p2pGroupSSID, p2pGroupPassphrase, nil
}

// P2PGroupCreate brings up an auto-provisioned WiFi Direct group and assumes its ownership
// (makes the host GO).
func (r *Runner) P2PGroupCreate(ctx context.Context, ipr *ip.Runner, ops ...p2p.GroupOption) (iface,
	ssid, key string, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	timeoutCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	const wpaMonitorStopTimeout = 5 * time.Second
	wpaMonitor := r.NewWPAMonitor()
	stop, ctx, err := wpaMonitor.StartWPAMonitor(timeoutCtx, wpaMonitorStopTimeout)
	if err != nil {
		return "", "", "", errors.Wrap(err, "failed to start wpa monitor")
	}
	defer stop()

	// Add a P2P group owner (GO).
	if err := r.p2pGroupAdd(ctx, ops...); err != nil {
		return "", "", "", err
	}

	p2pGOIface, p2pGroupSSID, p2pGroupPassphrase, err := r.p2pGroupStartedWait(ctx, wpaMonitor)
	if err != nil {
		return "", "", "", err
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			retErr = errors.Join(retErr, r.p2pFlush(ctx))
			retErr = errors.Join(retErr, r.p2pGroupRemove(ctx, p2pGOIface))
		}
	}(cleanupCtx)

	if err := ipr.SetLinkUp(ctx, p2pGOIface); err != nil {
		return "", "", "", err
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			retErr = errors.Join(retErr, ipr.SetLinkDown(ctx, p2pGOIface))
		}
	}(cleanupCtx)

	if err := ipr.AddIP(ctx, p2pGOIface, net.ParseIP(utils.P2PGOIPAddress), 24); err != nil {
		return "", "", "", err
	}

	testing.ContextLogf(ctx, "P2P Group owner (GO) %s: Configured on %s", p2pGroupSSID, p2pGOIface)

	return p2pGOIface, p2pGroupSSID, p2pGroupPassphrase, nil
}

// P2PGroupConnect connects to an existing P2P Group.
func (r *Runner) P2PGroupConnect(ctx context.Context, ipr *ip.Runner,
	ssid, key string, ops ...p2p.GroupOption) (iface string, netID int, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := r.DiscoverNetwork(timeoutCtx, string(ssid)); err != nil {
		return "", -1, err
	}

	const wpaMonitorStopTimeout = 5 * time.Second
	wpaMonitor := r.NewWPAMonitor()
	stop, timeoutCtx, err := wpaMonitor.StartWPAMonitor(timeoutCtx, wpaMonitorStopTimeout)
	if err != nil {
		return "", -1, errors.Wrap(err, "failed to start wpa monitor")
	}
	defer stop()

	networkID, err := r.p2pAddNetwork(timeoutCtx, ssid, key)
	if err != nil {
		return "", -1, err
	}
	ops = append(ops, p2p.SetNetworkID(networkID))

	// Add a p2p group owner (GO).
	if err := r.p2pGroupAdd(timeoutCtx, ops...); err != nil {
		return "", -1, err
	}

	p2pClientIface, p2pGroupSSID, p2pGroupPassphrase, err := r.p2pGroupStartedWait(timeoutCtx, wpaMonitor)
	if err != nil {
		return "", -1, err
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			// Best-effort cleanup.
			retErr = errors.Join(retErr, r.p2pFlush(ctx))
			retErr = errors.Join(retErr, r.p2pGroupRemove(ctx, p2pClientIface))
		}
	}(cleanupCtx)

	if p2pGroupSSID != string(ssid) || p2pGroupPassphrase != string(key) {
		return "", -1, errors.Errorf("P2P Group brought up with a different parameters, got (%q/%q), want (%q/%q)",
			p2pGroupSSID, p2pGroupPassphrase, string(ssid), string(key))
	}

	if err := ipr.SetLinkUp(timeoutCtx, p2pClientIface); err != nil {
		return "", -1, err
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			retErr = errors.Join(retErr, ipr.SetLinkDown(ctx, p2pClientIface))
		}
	}(cleanupCtx)

	if err := ipr.AddIP(timeoutCtx, p2pClientIface, net.ParseIP(utils.P2PClientIPAddress), 24); err != nil {
		return "", -1, err
	}

	testing.ContextLogf(timeoutCtx, "P2P Group client (GO) %s: Connected", p2pGroupSSID)

	return p2pClientIface, networkID, nil
}

// P2PGroupDelete lets GO tear down the WiFi Direct group.
func (r *Runner) P2PGroupDelete(ctx context.Context, ipr *ip.Runner,
	iface string) (retErr error) {
	retErr = errors.Join(retErr, ipr.DeleteIP(ctx, iface, net.ParseIP(utils.P2PGOIPAddress), 24))
	retErr = errors.Join(retErr, r.p2pGroupRemove(ctx, iface))
	retErr = errors.Join(retErr, r.p2pFlush(ctx))

	if retErr == nil {
		testing.ContextLog(ctx, "P2P Group owner (GO): Deconfigured")
	}
	return
}

// P2PGroupDisconnect Disconnects client from the WiFi Direct group.
func (r *Runner) P2PGroupDisconnect(ctx context.Context, ipr *ip.Runner,
	iface string, netID int) (retErr error) {
	retErr = errors.Join(retErr, ipr.DeleteIP(ctx, iface, net.ParseIP(utils.P2PClientIPAddress), 24))
	retErr = errors.Join(retErr, ipr.SetLinkDown(ctx, iface))
	retErr = errors.Join(retErr, r.p2pGroupRemove(ctx, iface))
	retErr = errors.Join(retErr, r.p2pFlush(ctx))
	retErr = errors.Join(retErr, r.RemoveNetwork(ctx, netID))

	if retErr == nil {
		testing.ContextLog(ctx, "P2P Client: Deconfigured")
	}
	return
}
