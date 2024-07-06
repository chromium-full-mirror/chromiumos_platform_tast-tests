// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type vpnDNSTestCase struct {
	vpnType vpn.Type
	isIPv6  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VPNDNS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that DNS config pushed by VPN servers are correctly applied",
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
			Name: "ikev2_ipv4",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeIKEv2,
				isIPv6:  false,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "ikev2_ipv6",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeIKEv2,
				isIPv6:  true,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeL2TPIPsec,
				isIPv6:  false,
			},
		}, {
			Name: "openvpn_ipv4",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeOpenVPN,
				isIPv6:  false,
			},
		}, {
			Name: "openvpn_ipv6",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeOpenVPN,
				isIPv6:  true,
			},
		}, {
			Name: "wireguard_ipv4",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeWireGuard,
				isIPv6:  false,
			},
			ExtraSoftwareDeps: []string{"wireguard"},
		}, {
			Name: "wireguard_ipv6",
			Val: vpnDNSTestCase{
				vpnType: vpn.TypeWireGuard,
				isIPv6:  true,
			},
			ExtraSoftwareDeps: []string{"wireguard"},
		},
		},
	})
}

func VPNDNS(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Dump network info on failure.
	errorHandler := dumputil.CreateErrorHandler(cleanupCtx)
	s.AttachErrorHandlers(errorHandler, errorHandler)

	// Set up test topology:
	// DUT---router---server (w/DNS: v?.foo.bar)
	//            |---vpn (w/DNS: domain.private)
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
	if err := vpnServer.ConnectToRouterWithPool(ctx, testEnv.Router, testEnv.Pool); err != nil {
		s.Fatal("Failed to connect vpn server to router: ", err)
	}
	defer func(ctx context.Context) {
		if err := vpnServer.Cleanup(ctx); err != nil {
			s.Error("Failed to tear down vpn server env: ", err)
		}
	}(cleanupCtx)

	const privateDomain = "domain.private"
	const privateDomainAddr = "203.0.113.33"
	dnsmasqOnServer := dnsmasq.New(
		dnsmasq.WithResolveHost(privateDomain, net.ParseIP(privateDomainAddr)),
		dnsmasq.WithAllInterfaces(),
	)
	if err := vpnServer.StartServer(ctx, "dnsmasq", dnsmasqOnServer); err != nil {
		s.Fatal("Failed to start dnsmasq on vpn server: ", err)
	}

	// Wait for veth to be online and server is reachable by IPv4 before
	// connecting to VPN. Since in WireGuard we don't really verify the VPN
	// connection before updating the service state to online in shill, it's
	// possible that IPv4 underlay network is ready after that, which causes test
	// flaky.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for service online: ", err)
	}
	addrs, err := vpnServer.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get physical addrs from vpn env: ", err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, addrs.IPv4Addr.String(), "root", 10*time.Second); err != nil {
		s.Fatal("Failed to verify physical connectivity to vpn env: ", err)
	}

	// Start the VPN connection. The VPN overlay will always be dual-stack (except
	// for L2TP/IPsec), and the configured DNS server address will be IPv4 or
	// IPv6-only depending on the test case. We mainly care about if the DNS can
	// be configured properly on DUT instead of the detailed resolve result, so
	// the DNS server in the test will always only return A records.
	testCase := s.Param().(vpnDNSTestCase)
	opts := []vpn.Option{
		vpn.WithIPType(vpn.IPTypeIPv4AndIPv6),
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
	}
	if testCase.isIPv6 {
		opts = append(opts, vpn.WithDNSUseDefaultIPv6())
	} else {
		opts = append(opts, vpn.WithDNSUseDefaultIPv4())
	}
	conn, err := vpn.StartConnection(ctx, vpnServer,
		testCase.vpnType,
		opts...,
	)
	if err != nil {
		s.Fatal("Failed to start VPN connection: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up vpn connection: ", err)
		}
	}()

	if err := ping.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping server overlay %s: %v", conn.Server.OverlayIPv4, err)
	}

	// Verify the VPN DNS config.
	networkConfig, err := conn.Service().GetNetworkConfig(ctx)
	if err != nil {
		s.Fatal("Failed to get NetworkConfig on VPN service")
	}
	// Always log the name servers configured on the service.
	s.Log("Got VPN DNS configuration: ", networkConfig.NameServers)
	if testCase.isIPv6 {
		if len(networkConfig.IPv4NameServers()) != 0 || len(networkConfig.IPv6NameServers()) == 0 {
			s.Fatal("Unexpected VPN DNS configuration, want only IPv6 name servers")
		}
	} else {
		if len(networkConfig.IPv4NameServers()) == 0 || len(networkConfig.IPv6NameServers()) != 0 {
			s.Fatal("Unexpected VPN DNS configuration, want only IPv4 name servers")
		}
	}

	// Verify that user and system traffic are using correct DNS correspondingly.
	// The first verification is relaxed with a timeout for dnsproxy to finish initialization.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyDNSResolve(ctx, "chronos", privateDomain, true, privateDomainAddr)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Error("DNS verification failure: ", err)
	}
	if err := dns.VerifyDNSResolve(ctx, "root", privateDomain, false, ""); err != nil {
		s.Error("DNS verification failure: ", err)
	}
	if err := dns.VerifyDNSResolve(ctx, "root", routing.TestDomainNameV4, true, ""); err != nil {
		s.Error("DNS verification failure: ", err)
	}
	if err := dns.VerifyDNSResolve(ctx, "chronos", routing.TestDomainNameV4, false, ""); err != nil {
		s.Error("DNS verification failure: ", err)
	}
}
