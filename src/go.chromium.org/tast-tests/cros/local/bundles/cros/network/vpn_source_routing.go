// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VPNSourceRouting,
		Desc:         "Verify traffic from different sources is routed to the correct interface",
		Contacts:     []string{"cros-networking@google.com", "garrick@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"ikev2"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// VPNSourceRouting verifies that traffic from different user sources is routed
// correctly, either into the VPN, or directly onto the underlying physical
// interface.
func VPNSourceRouting(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}()

	// Use Server1 for setting up VPN server, and Server2 for verify physical
	// traffic.
	vpnEnv := networkEnv.Server1
	physicalEnv := networkEnv.Server2

	addrs, err := physicalEnv.WaitForVethInAddrs(ctx, true, false)
	if err != nil {
		s.Fatal("Failed to get server addrs: ", err)
	}
	saddr := addrs.IPv4Addr.String()

	// Establish a VPN on one of the servers.
	conn, err := vpn.StartConnection(ctx, vpnEnv, vpn.TypeIKEv2)
	if err != nil {
		s.Fatal("Failed to connect vpn: ", err)
	}
	defer conn.Cleanup(cleanupCtx)
	vaddr := conn.Server.OverlayIPv4

	test := func(user, goodIP, badIP string) {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, goodIP, user, 10*time.Second); err != nil {
			s.Errorf("User %s failed to ping %v: %v", user, goodIP, err)
		}
		if err := ping.ExpectPingFailure(ctx, badIP, user); err != nil {
			s.Errorf("User %s able to ping %v: %v", user, badIP, err)
		}
	}

	// Verify that traffic from certain system user sources will egress on
	// the physical network and not the VPN.
	for _, u := range []string{"root", "dns-proxy"} {
		test(u, saddr, vaddr)
	}
	// Verify that traffic from other user sources should egress on the VPN
	// This list is not comprehensive. See patchpanel/routing_service.h for
	// additional details.
	for _, u := range []string{"chronos", "debugd", "kerberosd", "pluginvm"} {
		test(u, vaddr, saddr)
	}
}
