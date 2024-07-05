// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Network represents a common network topology used by VPN tests. See the
// comments for CreateNetworkTopology() for more detail.
type Network struct {
	manager *shill.Manager
	pool    *subnet.Pool

	Router  *virtualnet.Env
	Server1 *virtualnet.Env
	Server2 *virtualnet.Env

	privateEnv *virtualnet.Env
}

// CreateNetworkTopology sets up a network topology for VPN tests based on
// virtualnet package. As follows:
//
//	veth ----- Router --+-- Server1
//	                    |
//	                    +-- server2
//
// Notes:
//   - The caller should call TearDown() on the returned object to release the
//     resources after test is done.
//   - Portal detection will be disabled on all interfaces (not only the
//     created veth one), and be reset to default behavior in the TearDown(), no
//     matter the current setup.
//   - The created veth service will be set up the default service in shill by
//     tuning its priority, so need to be careful if the test also set up other
//     veth services using virtualnet package, to avoid any conflict.
//   - Before return, this function guarantee that the created veth service is
//     online and Server is reachable via both v4 and v6.
func CreateNetworkTopology(ctx context.Context) (*Network, error) {
	// Reserve some time for cleanup on failures. This function will start some
	// processes which are supposed to be kept running so do not defer the
	// cancel() here.
	cleanupCtx := ctx
	ctx, _ = ctxutil.Shorten(ctx, 5*time.Second)

	manager, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create manager proxy")
	}

	if err := manager.DisablePortalDetection(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to disable portal detection")
	}

	success := false
	network := &Network{manager: manager, pool: subnet.NewPool()}
	defer func(ctx context.Context) {
		if success {
			return
		}
		if err := network.TearDown(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to tear down VPN network topology: ", err)
		}
	}(cleanupCtx)

	var svc *shill.Service
	svc, network.Router, network.Server1, err = virtualnet.CreateRouterServerEnv(ctx, manager, network.pool, virtualnet.EnvOptions{
		Priority:   5,
		EnableDHCP: true,
		RAServer:   true,
		EnableDNS:  true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to start router and server env")
	}

	network.Server2, err = virtualnet.CreateEnv(ctx, "server2")
	if err != nil {
		return nil, errors.Wrap(err, "failed to start 2nd server env")
	}
	if err := network.Server2.ConnectToRouterWithPool(ctx, network.Router, network.pool); err != nil {
		return nil, errors.Wrap(err, "failed to connect 2nd server env to router")
	}

	if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to wait for service online")
	}

	// Verify that one server is reachable via both v4 and v6 to make sure routing
	// setup is ready.
	addrs, err := network.Server2.WaitForVethInAddrs(ctx, true, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get addresses from 2nd server")
	}
	for _, addr := range []net.IP{addrs.IPv4Addr, addrs.IPv6Addrs[0]} {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, addr.String(), "chronos", 20*time.Second); err != nil {
			return nil, errors.Wrap(err, "failed to verify server connectivity")
		}
	}

	success = true
	return network, nil
}

// CreatePrivateEnv creates a virtualnet.Env behind the vpnEnv which is
// exclusive to the vpn interface. On the forwarding path, packet from the vpn
// network can only reach this private network, and vice versa (note that the
// input/output path is not affected). This env can be used to verify the
// default route setup on DUT. The returned env will be owned by n and cleaned
// up on TearDown() so the caller should not clean it up directly.
func (n *Network) CreatePrivateEnv(ctx context.Context, server *Server, vpnEnv *virtualnet.Env) (*virtualnet.Env, error) {
	var err error
	n.privateEnv, err = CreatePrivateEnv(ctx, n.pool, server, vpnEnv)
	return n.privateEnv, err
}

// CreatePrivateEnv creates a virtualnet.Env behind the vpnEnv which is
// exclusive to the vpn interface. On the forwarding path, packet from the vpn
// network can only reach this private network, and vice versa (note that the
// input/output path is not affected). This env can be used to verify the
// default route setup on DUT. On success, the caller needs to call cleanup on
// the returned Env after it's no longer needed.
func CreatePrivateEnv(ctx context.Context, pool *subnet.Pool, server *Server, vpnEnv *virtualnet.Env) (*virtualnet.Env, error) {
	privateEnv, err := virtualnet.CreateEnv(ctx, "private")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create VPN private env")
	}

	success := false
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer func() {
		if success {
			return
		}
		if err := privateEnv.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to clean up env in defer handling: ", err)
		}
	}()

	if err := privateEnv.ConnectToRouterWithPool(ctx, vpnEnv, pool); err != nil {
		return nil, errors.Wrap(err, "failed to connect private env to VPN server env")
	}
	for _, iptablesCmd := range []string{"iptables", "ip6tables"} {
		cmds := [][]string{
			{iptablesCmd, "-I", "FORWARD", "-i", server.OverlayIfname, "-j", "DROP", "-w"},
			{iptablesCmd, "-I", "FORWARD", "-o", server.OverlayIfname, "-j", "DROP", "-w"},
			{iptablesCmd, "-I", "FORWARD", "-i", privateEnv.VethOutName, "-j", "DROP", "-w"},
			{iptablesCmd, "-I", "FORWARD", "-o", privateEnv.VethOutName, "-j", "DROP", "-w"},
			{iptablesCmd, "-I", "FORWARD", "-i", server.OverlayIfname, "-o", privateEnv.VethOutName, "-j", "ACCEPT", "-w"},
			{iptablesCmd, "-I", "FORWARD", "-o", server.OverlayIfname, "-i", privateEnv.VethOutName, "-j", "ACCEPT", "-w"},
		}
		for _, cmd := range cmds {
			if err := vpnEnv.RunWithoutChroot(ctx, cmd...); err != nil {
				return nil, errors.Wrap(err, "failed to install iptables rules to drop packets")
			}
		}
	}

	success = true
	return privateEnv, nil
}

// TearDown tears down the network topology.
func (n *Network) TearDown(ctx context.Context) error {
	var lastErr error

	for _, netEnv := range []*env.Env{n.Router, n.Server1, n.Server2, n.privateEnv} {
		if netEnv == nil {
			continue
		}
		if err := netEnv.Cleanup(ctx); err != nil {
			lastErr = errors.Wrapf(err, "failed to cleanup %s", netEnv.NetNSName)
			testing.ContextLog(ctx, "Failed to cleanup VPN network topology: ", lastErr)
		}
	}

	if err := n.manager.EnablePortalDetection(ctx); err != nil {
		lastErr = err
		testing.ContextLog(ctx, "Failed to cleanup VPN network topology: ", lastErr)
	}

	return lastErr
}
