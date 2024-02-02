// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VPNSplitRouting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that split routing works correctly for VPN",
		Contacts: []string{
			"cros-networking@google.com",
			"taoyl@google.com",
		},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "vpnEnvWithCerts",
		Params: []testing.Param{{
			Name: "openvpn",
			Val:  vpn.TypeOpenVPN,
		},
		},
	})
}

func VPNSplitRouting(ctx context.Context, s *testing.State) {
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

	conn, err := vpn.StartConnection(ctx, vpnEnv,
		s.Param().(vpn.Type),
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
		vpn.WithIPv4IncludedRoute(&net.IPNet{
			IP:   net.ParseIP("10.11.12.0"),
			Mask: net.CIDRMask(24, 32),
		}))
	if err != nil {
		s.Fatal("Failed to start VPN connection: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up vpn connection: ", err)
		}
	}()

	// Verifies VPN overlay reachability.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping server overlay %s: %v", conn.Server.OverlayIPv4, err)
	}

	// Verifies physical network (out of VPN included route) reachability.
	addrs, err := physicalEnv.WaitForVethInAddrs(ctx, true, false)
	if err != nil {
		s.Fatal("Failed to get server addrs: ", err)
	}
	phyAddr := addrs.IPv4Addr.String()
	if err := ping.ExpectPingSuccessWithTimeout(ctx, phyAddr, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping physical network host %s: %v", phyAddr, err)
	}
}
