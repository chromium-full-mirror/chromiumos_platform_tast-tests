// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/network/vpn"
	"chromiumos/tast/local/network/routing"
	"chromiumos/tast/local/network/virtualnet"
	"chromiumos/tast/testing"
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
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
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

func configFromType(vpnType vpn.Type, fixtureEnv vpn.FixtureEnv) *vpn.Config {
	if vpnType == vpn.TypeOpenVPN {
		return vpn.NewConfig(vpn.TypeOpenVPN,
			vpn.WithCertVals(fixtureEnv.CertVals),
			vpn.WithIPv4IncludedRoute(&net.IPNet{
				IP:   net.ParseIP("10.11.12.0"),
				Mask: net.CIDRMask(24, 32),
			}))
	}
	return nil
}

func VPNSplitRouting(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Set up test topology:
	// DUT---router---server
	//            |---vpnServer
	// Both server and vpnServer underlay address are out of VPN included route.
	testEnv := routing.NewSimpleNetworkEnv(true, true, true, true)
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up simple_net env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down simple_net env: ", err)
		}
	}(cleanupCtx)

	vpnServer, err := virtualnet.CreateEnv(ctx, "vpn")
	if err != nil {
		s.Fatal("Failed to setup vpn server env: ", err)
	}
	defer func(ctx context.Context) {
		if err := vpnServer.Cleanup(ctx); err != nil {
			s.Error("Failed to tear down vpn server env: ", err)
		}
	}(cleanupCtx)
	if err := vpnServer.ConnectToRouterWithPool(ctx, testEnv.Router, testEnv.Pool); err != nil {
		s.Fatal("Failed to connect vpn server to router: ", err)
	}

	vpnConfig := configFromType(s.Param().(vpn.Type), s.FixtValue().(vpn.FixtureEnv))
	conn, err := vpn.NewConnectionWithEnvs(ctx, *vpnConfig, vpnServer, nil)
	if err != nil {
		s.Fatal("Failed to create vpn connection object: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up vpn connection: ", err)
		}
	}()

	if err := conn.SetUp(ctx); err != nil {
		s.Fatal("Failed to setup VPN server: ", err)
	}

	// Wait for veth to be online then start connecting to VPN.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		s.Error("Failed to wait for service online: ", err)
	}

	connected, err := conn.Connect(ctx)
	if err != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	} else if !connected {
		s.Fatal("Failed to connect to VPN server: the service state changed to failure")
	}

	// Verifies VPN overlay reachability.
	if err := routing.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping server overlay %s: %v", conn.Server.OverlayIPv4, err)
	}

	// Verifies physical network (out of VPN included route) reachability.
	addrs, err := testEnv.Server.WaitForVethInAddrs(ctx, true, false)
	if err != nil {
		s.Fatal("Failed to get server addrs: ", err)
	}
	phyAddr := addrs.IPv4Addr.String()
	if err := routing.ExpectPingSuccessWithTimeout(ctx, phyAddr, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping physical network host %s: %v", phyAddr, err)
	}

}
