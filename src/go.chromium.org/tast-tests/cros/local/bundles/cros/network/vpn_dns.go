// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

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
			Name: "openvpn",
			Val:  vpn.TypeOpenVPN,
		}, {
			Name:              "ikev2",
			Val:               vpn.TypeIKEv2,
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec",
			Val:  vpn.TypeL2TPIPsec,
		}, {
			Name:              "wireguard",
			Val:               vpn.TypeWireGuard,
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

	conn, err := vpn.StartConnection(ctx, vpnServer,
		s.Param().(vpn.Type),
		vpn.WithCertVals(s.FixtValue().(vpn.FixtureEnv).CertVals),
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

	// Verify that user and system traffic are using correct DNS correspondingly.
	// The first verification is relaxed with a timeout for dnsproxy to finish initialization.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return verifyDNS(ctx, "chronos", privateDomain, true, privateDomainAddr)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Error("DNS verification failure: ", err)
	}
	if err := verifyDNS(ctx, "root", privateDomain, false, ""); err != nil {
		s.Error("DNS verification failure: ", err)
	}
	if err := verifyDNS(ctx, "root", routing.TestDomainNameV4, true, ""); err != nil {
		s.Error("DNS verification failure: ", err)
	}
	if err := verifyDNS(ctx, "chronos", routing.TestDomainNameV4, false, ""); err != nil {
		s.Error("DNS verification failure: ", err)
	}
}

// verifyDNS verifies whether the name resolution of |domain| by |user| is as expected.
//
//	expectResolvable == false: wanted non-resolavable
//	expectResolvable == true, len(expectedIP) != 0: wanted resolvable into expectedIP
//	expectResolvable == true, len(expectedIP) == 0: wanted resolvable (into any IP)
func verifyDNS(ctx context.Context, user, domain string, expectResolvable bool, expectedIP string) error {
	out, err := testexec.CommandContext(ctx, "sudo", "-u", user, "/usr/local/bin/dig", "+short", domain, "+tries=2", "+timeout=2").Output()
	if err != nil {
		return errors.Wrapf(err, "running dig %s as user %s failed:", domain, user)
	}

	trimmedOut := strings.TrimSpace(string(out))
	if len(trimmedOut) == 0 {
		if expectResolvable {
			if len(expectedIP) != 0 {
				return errors.Errorf("As user %s, %s cannot be resolved, want: %s", user, domain, expectedIP)
			}
			return errors.Errorf("As user %s, %s cannot be resolved, want: resolvable", user, domain)
		}
		testing.ContextLogf(ctx, "As user %s, %s cannot be resolved, as expected", user, domain)
		return nil
	}

	if !expectResolvable {
		return errors.Errorf("As user %s, %s resolved into %s, want: non-resolvable", user, domain, trimmedOut)
	}
	if len(expectedIP) != 0 && trimmedOut != expectedIP {
		return errors.Errorf("As user %s, %s resolved into %s, want %s", user, domain, trimmedOut, expectedIP)
	}
	testing.ContextLogf(ctx, "As user %s, %s resolved into %s", user, domain, trimmedOut)
	return nil
}
