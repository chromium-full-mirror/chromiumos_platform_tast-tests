// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/guestos"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
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
		Func:     DNSProxyOverVPN,
		Desc:     "Ensure that DNS proxies are working correctly over VPN",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:release-health", "release-health_network"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream", "ikev2"},
		Timeout:      2 * time.Minute,
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
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHAutomatic,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHAlwaysOn,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted",
		}, {
			Name: "crostini_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHOff,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBookworm",
		}, {
			Name: "crostini_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHAutomatic,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBookworm",
		}, {
			Name: "crostini_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHAlwaysOn,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBookworm",
		}, {
			Name: "root_ns_chrome_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode:   dns.DoHOff,
				chrome: true,
			},
			ExtraAttr: []string{"informational"},
			Fixture:   fixture.ChromeLoggedInWithRootNsDnsProxy,
		}, {
			Name: "root_ns_chrome_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode:   dns.DoHAutomatic,
				chrome: true,
			},
			ExtraAttr: []string{"informational"},
			Fixture:   fixture.ChromeLoggedInWithRootNsDnsProxy,
		}, {
			Name: "root_ns_chrome_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode:   dns.DoHAlwaysOn,
				chrome: true,
			},
			ExtraAttr: []string{"informational"},
			Fixture:   fixture.ChromeLoggedInWithRootNsDnsProxy,
		}, {
			Name: "root_ns_arc_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHOff,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			ExtraAttr:         []string{"informational"},
			Fixture:           "arcBootedWithRootNsDnsProxy",
		}, {
			Name: "root_ns_arc_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHAutomatic,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			ExtraAttr:         []string{"informational"},
			Fixture:           "arcBootedWithRootNsDnsProxy",
		}, {
			Name: "root_ns_arc_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode: dns.DoHAlwaysOn,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			ExtraAttr:         []string{"informational"},
			Fixture:           "arcBootedWithRootNsDnsProxy",
		}, {
			Name: "root_ns_crostini_doh_off",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHOff,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			ExtraAttr:         []string{"informational"},
			Fixture:           "crostiniBookwormWithRootNsDnsProxy",
		}, {
			Name: "root_ns_crostini_doh_automatic",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHAutomatic,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			ExtraAttr:         []string{"informational"},
			Fixture:           "crostiniBookwormWithRootNsDnsProxy",
		}, {
			Name: "root_ns_crostini_doh_always_on",
			Val: dnsProxyOverVPNTestParams{
				mode:     dns.DoHAlwaysOn,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			ExtraAttr:         []string{"informational"},
			Fixture:           "crostiniBookwormWithRootNsDnsProxy",
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

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewDumpARCOnFailureHook(a),
		testhooks.NewDumpCrostiniOnFailureHook(cont),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

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
		vpnServer.Cleanup(cleanupCtx)
	}()

	// Wait for routing setup ready for guests. Host is verified in
	// connectToVPN().
	if params.arc {
		if err := arcnet.ExpectPingSuccess(ctx, a, "vpn", conn.Server.OverlayIPv4); err != nil {
			s.Fatal("Failed to wait for ARC able to use VPN: ", err)
		}
	}
	if params.crostini {
		if err := guestos.PingWithRetryAndTimeout(ctx, cont, conn.Server.OverlayIPv4, 10*time.Second); err != nil {
			s.Fatal("Failed to wait for Crostini able to use VPN: ", err)
		}
	}

	// Wait for the updated network configuration (VPN) to be propagated to the proxy.
	if err := waitUntilNATIptablesConfigured(ctx); err != nil {
		s.Fatal("iptables NAT output is not fully configured: ", err)
	}

	// By default, DNS query should work over VPN.
	var defaultTC []dns.ProxyTestCase
	if params.chrome {
		defaultTC = []dns.ProxyTestCase{{Client: dns.System}, {Client: dns.User}, {Client: dns.Chronos}}
	} else if params.arc {
		defaultTC = []dns.ProxyTestCase{{Client: dns.ARC}}
	} else if params.crostini {
		defaultTC = []dns.ProxyTestCase{{Client: dns.Crostini}}
	}
	for _, tc := range defaultTC {
		if err := tc.Run(ctx, nil /* chrome */, a, cont, dns.NewQueryOptions()); err != nil {
			s.Error("Failed DNS query check in the default setup: ", err)
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
		vpnBlockedTC = []dns.ProxyTestCase{{Client: dns.System}, {Client: dns.User, ExpectErr: true}, {Client: dns.Chronos, ExpectErr: true}}
	} else if params.arc {
		vpnBlockedTC = []dns.ProxyTestCase{{Client: dns.ARC, ExpectErr: true}}
	} else if params.crostini {
		vpnBlockedTC = []dns.ProxyTestCase{{Client: dns.Crostini, ExpectErr: true}}
	}

	// Block DNS queries over VPN through iptables.
	if errs := dns.NewVPNBlock(vpnServer.Env.NetNSName).Run(ctx, func(ctx context.Context) {
		for _, tc := range vpnBlockedTC {
			if err := tc.Run(ctx, nil /* chrome */, a, cont, dns.NewQueryOptions()); err != nil {
				s.Error("Failed DNS query check: ", err)
			}
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
		if !bytes.Equal(lastRules, rules) || !bytes.Equal(lastRules6, rules6) {
			lastRules = rules
			lastRules6 = rules6
			return errors.New("iptables NAT rules are still being configured")
		}
		return nil
	}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 15 * time.Second})
}

// connectToVPN creates a VPN server and connects to it.
// On success, the caller is responsible to cleanup the created server and VPN connection.
func connectToVPN(ctx context.Context, pool *subnet.Pool, router *env.Env, httpsCerts *certs.Certs) (*dns.Server, *vpn.Connection, error) {
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
		server.Cleanup(ctx)
	}()

	// Connect to VPN.
	conn, err := vpn.StartConnection(ctx, server.Env, vpn.TypeIKEv2)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start VPN connection")
	}

	success = true
	return server, conn, nil
}
