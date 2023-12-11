// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     OpenVPNTopology,
		Desc:     "Verify that shill respects the topology option in OpenVPN properly",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:network", "network_platform_unstable", "group:hw_agnostic"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      "vpnEnvWithCerts",
		Params: []testing.Param{{
			Name: "net30",
			Val:  vpn.OpenVPNTopologyNet30,
		}, {
			Name: "p2p",
			Val:  vpn.OpenVPNTopologyP2P,
		}, {
			Name: "subnet",
			Val:  vpn.OpenVPNTopologySubnet,
		}},
	})
}

// OpenVPNTopology verifies that the VPN server (overlay address) is reachable
// via VPN for all kinds of topology. Default route is turned off in the test to
// make sure shill has installed the specific routes for the subnet or peer.
func OpenVPNTopology(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
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

	topology := s.Param().(vpn.OpenVPNTopology)
	conn, err := vpn.StartConnection(ctx, vpnEnv,
		vpn.TypeOpenVPN,
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
		vpn.WithOpenVPNTopology(topology))
	if err != nil {
		s.Fatal("Failed to create VPN server: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to stop VPN server: ", err)
		}
	}()

	privateEnv, err := networkEnv.CreatePrivateEnv(ctx, conn.Server, vpnEnv)
	if err != nil {
		s.Fatal("Failed to create VPN private env: ", err)
	}

	physicalEnvIPs, err := physicalEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get IPs in physical env: ", err)
	}
	privateEnvIPs, err := privateEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get IPs in private env: ", err)
	}

	type testCase struct {
		ip   string
		user string
		role string
	}
	reachableIPs := []testCase{
		// We should install routes for the overlay server address properly.
		{conn.Server.OverlayIPv4, "chronos", "server IPv4"},
		// This is split routing setup, physical address should not be blocked.
		{physicalEnvIPs.IPv4Addr.String(), "chronos", "physical IPv4"},
	}
	unreachableIPs := []testCase{
		// Private env is not in the same subnet with the VPN server, it should not
		// be reachable in all cases.
		{privateEnvIPs.IPv4Addr.String(), "chronos", "private IPv4"},
		{privateEnvIPs.IPv4Addr.String(), "root", "private IPv4"},
	}
	for _, tc := range reachableIPs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, tc.ip, tc.user, 10*time.Second); err != nil {
			s.Errorf("Failed to ping %s %s as %s: %v", tc.role, tc.ip, tc.user, err)
		}
	}
	for _, tc := range unreachableIPs {
		if err := ping.ExpectPingFailure(ctx, tc.ip, tc.user); err != nil {
			s.Errorf("Unexpected ping success for %s %s as %s: %v", tc.role, tc.ip, tc.user, err)
		}
	}
}
