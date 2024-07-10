// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/crostini"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type dnsProxyOverVPNTestParams struct {
	mode     dns.DoHMode
	chrome   bool
	arc      bool
	crostini bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxyOverVPN,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensure that DNS proxies are working correctly over VPN",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com", "garrick@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      7 * time.Minute,
		Params: []testing.Param{{
			Name: "chrome_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode:   dns.DoHOff,
				chrome: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "chrome_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode:   dns.DoHAutomatic,
				chrome: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "chrome_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode:   dns.DoHAlwaysOn,
				chrome: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "arc_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHOff,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHAutomatic,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHAlwaysOn,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "arcBooted",
		}, {
			Name: "crostini_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHOff,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBullseye",
		}, {
			Name: "crostini_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHAutomatic,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBullseye",
		}, {
			Name: "crostini_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHAlwaysOn,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBullseye",
		}},
	})
}

// DNSProxyOverVPN tests DNS functionality with DNS proxy active.
// There are 3 parts to this test:
// 1. Ensuring that DNS queries over VPN are successful.
// 2. Ensuring that DNS queries (except from system) are routed properly through VPN by blocking VPN DNS ports, expecting the queries to fail.
// 3. Ensuring that DNS queries (except from system) are not using DNS-over-HTTPS when a VPN is on.
func DNSProxyOverVPN(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	var (
		a    *arc.ARC
		cont *vm.Container
	)

	params := s.Param().(dnsProxyOverVPNTestParams)
	if params.arc {
		a = s.FixtValue().(*arc.PreData).ARC
	} else if params.crostini {
		cont = s.FixtValue().(crostini.FixtureData).Cont
	}

	if params.crostini {
		if err := dns.VerifyDigInstalledInContainer(ctx, cont); err != nil {
			s.Fatal("Failed to install dig in container: ", err)
		}
	}

	if params.arc {
		// Hide unused ethernet to avoid ARC's limitation.
		m, err := shill.NewManager(ctx)
		if err != nil {
			s.Fatal("Failed to create shill client: ", err)
		}
		restoreEthernet, err := arcnet.HideUnusedEthernet(ctx, m)
		if err != nil {
			s.Fatal("Failed to hide unused ethernet: ", err)
		}
		defer restoreEthernet(cleanupCtx)
	}

	// Set up virtualnet environment.
	pool := subnet.NewPool()
	env, err := dns.NewEnv(ctx, pool)
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	// Create and connect to a VPN server.
	vpnServer, conn, err := connectToVPN(ctx, pool, env.Router, env.Certs)
	if err != nil {
		s.Fatal("Failed to connect to VPN: ", err)
	}
	defer func() {
		if err := conn.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up VPN connection: ", err)
		}
		if err := vpnServer.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up VPN server: ", err)
		}
	}()

	// Wait for the updated network configuration (VPN) to be propagated to the proxy.
	if err := waitUntilNATIptablesConfigured(ctx); err != nil {
		s.Fatal("iptables NAT output is not fully configured: ", err)
	}

	// By default, DNS query should work over VPN.
	var defaultTC []dns.ProxyTestCase
	if params.chrome {
		defaultTC = []dns.ProxyTestCase{{Client: dns.System}, {Client: dns.User}, {Client: dns.Chrome}}
	} else if params.arc {
		defaultTC = []dns.ProxyTestCase{{Client: dns.ARC}}
	} else if params.crostini {
		defaultTC = []dns.ProxyTestCase{{Client: dns.Crostini}}
	}
	if errs := dns.TestQueryDNSProxy(ctx, defaultTC, a, cont, dns.NewQueryOptions()); len(errs) != 0 {
		for _, err := range errs {
			s.Error("Failed DNS query check: ", err)
		}
	}

	// Toggle plain-text DNS or secureDNS depending on test parameter.
	cleanup, err := dns.SetDoHModeViaShill(ctx, params.mode, dns.ExampleDoHProvider)
	if err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// DNS queries that should be routed through VPN should fail if DNS queries on the VPN server are blocked.
	// System traffic bypass VPN, this is to allow things such as updates and crash reports to always work.
	// On the other hand, other traffic (Chrome, ARC, etc.) should always go through VPN.
	var vpnBlockedTC []dns.ProxyTestCase
	if params.chrome {
		vpnBlockedTC = []dns.ProxyTestCase{{Client: dns.System}, {Client: dns.User, ExpectErr: true}, {Client: dns.Chrome, ExpectErr: true}}
	} else if params.arc {
		vpnBlockedTC = []dns.ProxyTestCase{{Client: dns.ARC, ExpectErr: true}}
	} else if params.crostini {
		vpnBlockedTC = []dns.ProxyTestCase{{Client: dns.Crostini, ExpectErr: true}}
	}

	// Block DNS queries over VPN through iptables.
	if errs := dns.NewVPNBlock(vpnServer.NetNSName).Run(ctx, func(ctx context.Context) {
		if errs := dns.TestQueryDNSProxy(ctx, vpnBlockedTC, a, cont, dns.NewQueryOptions()); len(errs) != 0 {
			s.Error("Failed DNS query check: ", errs)
		}
	}); len(errs) > 0 {
		s.Fatal("Failed to block DNS over VPN: ", errs)
	}
}

// waitUntilNATIptablesConfigured waits until the NAT rule output of iptables is fully configured.
// Whenever a network setting related to DNS is changed, DNS proxy updates iptables by deleting the old rule and creating a new rule.
// This function confirms that the expected changes have been fully propagated by periodically comparing the rules until no differences are detected between successive iterations.
func waitUntilNATIptablesConfigured(ctx context.Context) error {
	var lastRules, lastRules6 []byte
	return testing.Poll(ctx, func(ctx context.Context) error {
		rules, err := testexec.CommandContext(ctx, "iptables", "-t", "nat", "-S", "-w").Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to execute iptables")
		}
		rules6, err := testexec.CommandContext(ctx, "ip6tables", "-t", "nat", "-S", "-w").Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to execute ip6tables")
		}
		if bytes.Compare(lastRules, rules) != 0 || bytes.Compare(lastRules6, rules6) != 0 {
			lastRules = rules
			lastRules6 = rules6
			return errors.New("iptables NAT rules are still being configured")
		}
		return nil
	}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 15 * time.Second})
}

// connectToVPN creates a VPN server and connects to it.
// On success, the caller is responsible to cleanup the created server and VPN connection.
func connectToVPN(ctx context.Context, pool *subnet.Pool, router *env.Env, httpsCerts *certs.Certs) (*env.Env, *vpn.Connection, error) {
	serverIPv4Subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to allocate v4 subnet")
	}
	serverIPv6Subnet, err := pool.AllocNextIPv6Subnet()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to allocate v6 subnet")
	}
	server, err := dns.NewServer(ctx, "vpnserver", serverIPv4Subnet, serverIPv6Subnet, router, httpsCerts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to set up server env")
	}

	success := false
	defer func() {
		if success {
			return
		}
		if err := server.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup server env: ", err)
		}
	}()

	// Connect to VPN.
	conn, err := vpn.StartConnection(ctx, server, vpn.TypeL2TPIPsec)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start VPN connection")
	}

	success = true
	return server, conn, nil
}
