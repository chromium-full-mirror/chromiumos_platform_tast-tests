// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/network/vpn"
	"chromiumos/tast/local/network/dumputil"
	"chromiumos/tast/local/network/routing"
	"chromiumos/tast/local/network/virtualnet"
	"chromiumos/tast/testing"
)

type vpnRoutingTestCase struct {
	vpnType               vpn.Type
	ipType                vpn.IPType // v4, v6, or dual-stack
	underlayIPIsOverlayIP bool       // use the same IP for overlay and underlay to simulate a weird setup
	wgTwoPeers            bool       // use two peers for WireGuard tests
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     VPNRouting,
		Desc:     "Ensure that routing works properly when a VPN is connected",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "ikev2_ipv4",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeIKEv2,
				ipType:  vpn.IPTypeIPv4,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_ipv6",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeIKEv2,
				ipType:  vpn.IPTypeIPv6,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_ipv4_ipv6",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeIKEv2,
				ipType:  vpn.IPTypeIPv4AndIPv6,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeL2TPIPsec,
			},
			Fixture: "vpnEnv",
		}, {
			Name: "l2tp_ipsec_evil",
			Val: vpnRoutingTestCase{
				vpnType:               vpn.TypeL2TPIPsec,
				underlayIPIsOverlayIP: true,
			},
			Fixture: "vpnEnv",
		}, {
			Name: "openvpn",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeOpenVPN,
			},
			Fixture: "vpnEnvWithCerts",
		}, {
			Name: "wireguard_ipv4",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeWireGuard,
				ipType:  vpn.IPTypeIPv4,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_ipv4_two_peers",
			Val: vpnRoutingTestCase{
				vpnType:    vpn.TypeWireGuard,
				ipType:     vpn.IPTypeIPv4,
				wgTwoPeers: true,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_ipv6",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeWireGuard,
				ipType:  vpn.IPTypeIPv6,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_ipv4_ipv6",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeWireGuard,
				ipType:  vpn.IPTypeIPv4AndIPv6,
			},
			Fixture:           "vpnEnv",
			ExtraSoftwareDeps: []string{"wireguard"},
		}},
	})
}

func VPNRouting(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Create envs for holding servers.
	routingEnv := routing.NewTestEnvWithoutResetProfile()
	if err := routingEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to setup routing env: ", err)
	}
	defer func() {
		if err := routingEnv.TearDown(cleanupCtx); err != nil {
			testing.ContextLog(ctx, "Failed to tear down routing env: ", err)
		}
	}()
	// Create another env and connect it to the router. This can be used to verify
	// if physical network is reachable.
	physicalEnv, err := virtualnet.CreateEnv(ctx, "phy")
	if err != nil {
		s.Fatal("Failed to setup env for verifying physical connection: ", err)
	}
	defer func() {
		if err := physicalEnv.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(ctx, "Failed to tear down physical env: ", err)
		}
	}()
	if err := physicalEnv.ConnectToRouterWithPool(ctx, routingEnv.BaseRouter, routingEnv.Pool); err != nil {
		s.Fatal("Failed to connect physical env to router: ", err)
	}

	// Verify physicalEnv can be reached by IPv6 before connecting VPN.
	physicalAddrs, err := physicalEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to addrs from physical env: ", err)
	}
	if err := routing.ExpectPingSuccessWithTimeout(ctx, physicalAddrs.IPv6Addrs[0].String(), "chronos", 10*time.Second); err != nil {
		s.Fatal("Cannot reach physical env by IPv6: ", err)
	}

	tc := s.Param().(vpnRoutingTestCase)
	opts := []vpn.Option{
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
		vpn.WithIPType(tc.ipType),
	}
	if tc.underlayIPIsOverlayIP {
		opts = append(opts, vpn.WithUnderlayIPIsOverlayIP())
	}
	config := vpn.NewConfig(tc.vpnType, opts...)

	server, err := vpn.StartServerWithConfig(ctx, routingEnv.BaseServer, config)
	if err != nil {
		s.Fatal("Failed to create VPN server: ", err)
	}
	defer func() {
		if err := server.Exit(cleanupCtx); err != nil {
			s.Error("Failed to stop VPN server: ", err)
		}
	}()

	// The second server is only for WireGuard.
	var secondServer *vpn.Server
	if tc.wgTwoPeers {
		secondServer, err = vpn.StartServerWithConfig(ctx, routingEnv.BaseRouter, config)
		if err != nil {
			s.Fatal("Failed to create second WireGuard server: ", err)
		}
		defer func() {
			if err := secondServer.Exit(cleanupCtx); err != nil {
				s.Error("Failed to stop second WireGuard server: ", err)
			}
		}()
	}

	service, err := vpn.ConfigureService(ctx, server, secondServer, config)
	if err != nil {
		s.Fatal("Failed to configure VPN service: ", err)
	}
	defer func() {
		if err := service.Remove(cleanupCtx); err != nil {
			s.Error("Failed to remove VPN service: ", err)
		}
	}()

	if err := service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to the VPN service: ", err)
	}

	connectErr := service.WaitForConnectedOrError(ctx)
	if err := dumputil.DumpNetworkInfo(ctx, "network_dump_after_vpn_connect.txt"); err != nil {
		testing.ContextLog(ctx, "Failed to dump network info after VPN connect")
	}
	if connectErr != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	}

	if tc.ipType == vpn.IPTypeIPv4 || tc.ipType == vpn.IPTypeIPv4AndIPv6 {
		if err := routing.ExpectPingSuccessWithTimeout(ctx, server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
			s.Fatalf("Failed to ping %s: %v", server.OverlayIPv4, err)
		}
		if secondServer != nil {
			if err := routing.ExpectPingSuccessWithTimeout(ctx, secondServer.OverlayIPv4, "chronos", 10*time.Second); err != nil {
				s.Fatalf("Failed to ping %s: %v", secondServer.OverlayIPv4, err)
			}
		}
	}
	if tc.ipType == vpn.IPTypeIPv6 || tc.ipType == vpn.IPTypeIPv4AndIPv6 {
		if err := routing.ExpectPingSuccessWithTimeout(ctx, server.OverlayIPv6, "chronos", 10*time.Second); err != nil {
			s.Fatalf("Failed to ping %s: %v", server.OverlayIPv6, err)
		}
		if secondServer != nil {
			if err := routing.ExpectPingSuccessWithTimeout(ctx, secondServer.OverlayIPv6, "chronos", 10*time.Second); err != nil {
				s.Fatalf("Failed to ping %s: %v", secondServer.OverlayIPv6, err)
			}
		}
	}

	// In IPv4-only case, IPv6 should be blackholed.
	if tc.ipType != vpn.IPTypeIPv4 {
		return
	}
	// TODO(b/257379393): WireGuard does not support this properly now.
	if tc.vpnType == vpn.TypeWireGuard {
		testing.ContextLog(ctx, "Skip IPv6 blocking check for WireGuard")
		return
	}
	if err := routing.ExpectPingFailure(ctx, physicalAddrs.IPv6Addrs[0].String(), "chronos"); err != nil {
		s.Fatal("IPv6 ping should fail: ", err)
	}
}
