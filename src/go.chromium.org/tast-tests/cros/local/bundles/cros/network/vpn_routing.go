// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
		Attr:         []string{"group:mainline", "group:network", "network_cq"},
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
			Name: "openvpn_ipv4",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeOpenVPN,
				ipType:  vpn.IPTypeIPv4,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_ipv6",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeOpenVPN,
				ipType:  vpn.IPTypeIPv6,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_ipv4_ipv6",
			Val: vpnRoutingTestCase{
				vpnType: vpn.TypeOpenVPN,
				ipType:  vpn.IPTypeIPv4AndIPv6,
			},
			Fixture:           "vpnEnvWithCerts",
			ExtraAttr:         []string{"group:cq-medium"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
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

	// Verify physicalEnv can be reached by IPv6 before connecting VPN.
	physicalAddrs, err := physicalEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to addrs from physical env: ", err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, physicalAddrs.IPv6Addrs[0].String(), "chronos", 10*time.Second); err != nil {
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

	server, err := vpn.StartServerWithConfig(ctx, vpnEnv, config)
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
		secondServerOpts := []vpn.Option{
			vpn.WithIPType(tc.ipType),
			vpn.WithWGServerSecondKeyPair(true),
			vpn.WithWGUsePSK(true),
		}
		secondServer, err = vpn.StartServer(ctx, networkEnv.Router, vpn.TypeWireGuard, secondServerOpts...)
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
		s.Fatal("Failed to connect to VPN server: ", connectErr)
	}

	privateEnv, err := networkEnv.CreatePrivateEnv(ctx, server, vpnEnv)
	if err != nil {
		s.Fatal("Failed to create VPN private env: ", err)
	}

	privateEnvIPs, err := privateEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get IPs in private env: ", err)
	}

	type ipAndRole struct {
		ip   string
		role string
	}
	var reachableIPs []ipAndRole

	// Note that wgTwoPeers is a split-routing setup, so only verify default route
	// if that is not enabled.
	if tc.ipType == vpn.IPTypeIPv4 || tc.ipType == vpn.IPTypeIPv4AndIPv6 {
		reachableIPs = append(reachableIPs, ipAndRole{server.OverlayIPv4, "server IPv4"})
		if tc.wgTwoPeers {
			reachableIPs = append(reachableIPs, ipAndRole{secondServer.OverlayIPv4, "second server IPv4"})
		} else {
			reachableIPs = append(reachableIPs, ipAndRole{privateEnvIPs.IPv4Addr.String(), "private IPv4"})
		}
	}
	if tc.ipType == vpn.IPTypeIPv6 || tc.ipType == vpn.IPTypeIPv4AndIPv6 {
		reachableIPs = append(reachableIPs, ipAndRole{server.OverlayIPv6, "server IPv6"})
		if tc.wgTwoPeers {
			reachableIPs = append(reachableIPs, ipAndRole{secondServer.OverlayIPv6, "second server IPv6"})
		} else {
			reachableIPs = append(reachableIPs, ipAndRole{privateEnvIPs.IPv6Addrs[0].String(), "private IPv6"})
		}
	}
	for _, ip := range reachableIPs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, ip.ip, "chronos", 10*time.Second); err != nil {
			s.Errorf("Failed to ping %s %s: %v", ip.role, ip.ip, err)
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
	if err := ping.ExpectPingFailure(ctx, physicalAddrs.IPv6Addrs[0].String(), "chronos"); err != nil {
		s.Fatal("IPv6 ping should fail: ", err)
	}
}
