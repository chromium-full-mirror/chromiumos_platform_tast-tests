// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"strings"
	"time"

	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/network/vpn"
	"chromiumos/tast/local/network/routing"
	"chromiumos/tast/local/network/virtualnet/dnsmasq"
	"chromiumos/tast/local/network/virtualnet/env"
	"chromiumos/tast/testing"
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
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "vpnShillResetWithChromeLoggedIn",
		Params: []testing.Param{{
			Name: "openvpn",
			Val: vpn.Config{
				Type:           vpn.TypeOpenVPN,
				AuthType:       vpn.AuthTypeCert,
				OpenVPNTLSAuth: true,
				PushDNS:        true,
			},
		}, {
			Name: "ikev2",
			Val: vpn.Config{
				Type:     vpn.TypeIKEv2,
				AuthType: vpn.AuthTypePSK,
				PushDNS:  true,
			},
			ExtraSoftwareDeps: []string{"ikev2"},
		}, {
			Name: "l2tp_ipsec",
			Val: vpn.Config{
				Type:     vpn.TypeL2TPIPsec,
				AuthType: vpn.AuthTypePSK,
				PushDNS:  true,
			},
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

	vpnServer := env.New("vpn")
	if err := vpnServer.SetUp(ctx); err != nil {
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

	vpnConfig := s.Param().(vpn.Config)
	vpnConfig.CertVals = s.FixtValue().(vpn.FixtureEnv).CertVals
	conn, err := vpn.NewConnectionWithEnvs(ctx, vpnConfig, vpnServer, nil)
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

	// Wait for veth to be online then start connecting to VPN
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		s.Error("Failed to wait for service online: ", err)
	}

	connected, err := conn.Connect(ctx)
	if err != nil {
		s.Fatal("Failed to connect to VPN server: ", err)
	} else if !connected {
		s.Fatal("Failed to connect to VPN server: the service state changed to failure")
	}

	if err := routing.ExpectPingSuccessWithTimeout(ctx, conn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping server overlay %s: %v", conn.Server.OverlayIPv4, err)
	}

	// Verify that user and system traffic are using correct DNS correspondingly
	if err := verifyDNS(ctx, "chronos", privateDomain, true, privateDomainAddr); err != nil {
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
	out, err := testexec.CommandContext(ctx, "sudo", "-u", user, "/usr/local/bin/dig", "+short", domain).Output()
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
