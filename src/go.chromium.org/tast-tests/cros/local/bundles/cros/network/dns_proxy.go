// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/network"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type dnsProxyTestParams struct {
	mode     dns.DoHMode
	chrome   bool
	arc      bool
	crostini bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensure that DNS proxies are working correctly",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com", "garrick@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      7 * time.Minute,
		Params: []testing.Param{{
			Name: "chrome_doh_off",
			Val: dnsProxyTestParams{
				mode:   dns.DoHOff,
				chrome: true,
			},
			ExtraAttr: []string{},
			Fixture:   "chromeLoggedIn",
		}, {
			Name: "chrome_doh_automatic",
			Val: dnsProxyTestParams{
				mode:   dns.DoHAutomatic,
				chrome: true,
			},
			ExtraAttr: []string{},
			Fixture:   "chromeLoggedIn",
		}, {
			Name: "chrome_doh_always_on",
			Val: dnsProxyTestParams{
				mode:   dns.DoHAlwaysOn,
				chrome: true,
			},
			ExtraAttr: []string{},
			Fixture:   "chromeLoggedIn",
		}, {
			Name: "arc_doh_off",
			Val: dnsProxyTestParams{
				mode: dns.DoHOff,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_automatic",
			Val: dnsProxyTestParams{
				mode: dns.DoHAutomatic,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_always_on",
			Val: dnsProxyTestParams{
				mode: dns.DoHAlwaysOn,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "arcBooted",
		}, {
			Name: "crostini_doh_off",
			Val: dnsProxyTestParams{
				mode:     dns.DoHOff,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBullseye",
		}, {
			Name: "crostini_doh_automatic",
			Val: dnsProxyTestParams{
				mode:     dns.DoHAutomatic,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBullseye",
		}, {
			Name: "crostini_doh_always_on",
			Val: dnsProxyTestParams{
				mode:     dns.DoHAlwaysOn,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBullseye",
		}},
	})
}

// DNSProxy tests DNS functionality with DNS proxy active.
// There are 2 parts to this test:
// 1. Ensuring that DNS queries are successful.
// 2. Ensuring that DNS queries are using proper mode (Off, Automatic, Always On) by blocking the expected ports, expecting the queries to fail.
func DNSProxy(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	var (
		a    *arc.ARC
		cont *vm.Container
	)

	params := s.Param().(dnsProxyTestParams)
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
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	if params.crostini {
		// Ensure connectivity is available.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return testexec.CommandContext(ctx, "/bin/ping", "-c1", "-w1", "8.8.8.8").Run()
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			s.Log("Failed to ping 8.8.8.8: ", err)
		}

		// Ensure connectivity is available inside Crostini's container.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return cont.Command(ctx, "ping", "-c1", "-w1", "8.8.8.8").Run()
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			s.Log("Failed to ping 8.8.8.8 from Crostini: ", err)
		}

		// Install dig in container.
		if err := dns.InstallDigInContainer(ctx, cont); err != nil {
			s.Fatal("Failed to install dig in container: ", err)
		}
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill client: ", err)
	}
	if params.arc {
		// Hide unused ethernet to avoid ARC's limitation.
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

	// Configure DoH mode via shill.
	cleanup, err := dns.SetDoHModeViaShill(ctx, params.mode, dns.ExampleDoHProvider)
	if err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Make sure that ARC gets the DNS proxy address before proceeding the tests.
	if params.arc {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return dns.VerifyARCNameservers(ctx, a)
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to wait for ARC to get the nameservers config: ", err)
		}
	}

	// By default, DNS query should work.
	var tc []dns.ProxyTestCase
	if params.chrome {
		tc = []dns.ProxyTestCase{{Client: dns.System}, {Client: dns.User}, {Client: dns.Chrome}}
	} else if params.arc {
		tc = []dns.ProxyTestCase{{Client: dns.ARC}}
	} else if params.crostini {
		tc = []dns.ProxyTestCase{{Client: dns.Crostini}}
	}
	if errs := dns.TestQueryDNSProxy(ctx, tc, a, cont, dns.NewQueryOptions()); len(errs) != 0 {
		for _, err := range errs {
			s.Error("Failed DNS query check: ", err)
		}
	}

	nss, err := dns.ProxyNamespaces(ctx)
	if err != nil {
		s.Fatal("Failed to get DNS proxy's network namespaces: ", err)
	}

	physIfs, err := network.PhysicalInterfaces(ctx)
	if err != nil {
		s.Fatal("Failed to get physical interfaces: ", err)
	}

	// Block plain-text or secure DNS through iptables.
	var blocks []*dns.Block
	switch params.mode {
	case dns.DoHAutomatic:
		// Confirm blocking plaintext still works (DoH preferred/used).
		blocks = append(blocks, dns.NewPlaintextBlock(nss, physIfs, ""))
		// Verify blocking HTTPS also works (fallback).
		blocks = append(blocks, dns.NewDoHBlock(nss, physIfs))
		// Chrome isn't tested since it manages it's own DoH flow.
		// Allow retry for automatic mode. This is needed because Do53 fallback is only done after a DoH failure.
		// For this case, the failure happens on the proxy's DoH timeout which might be longer than the client's timeout.
		// Allow the client to retry the query. It is expected for the DoH server to be invalidated by then.
		if params.chrome {
			tc = []dns.ProxyTestCase{{Client: dns.System, AllowRetry: true}, {Client: dns.User, AllowRetry: true}}
		} else if params.arc {
			tc = []dns.ProxyTestCase{{Client: dns.ARC, AllowRetry: true}}
		} else if params.crostini {
			tc = []dns.ProxyTestCase{{Client: dns.Crostini, AllowRetry: true}}
		}
	case dns.DoHOff:
		// Verify blocking plaintext causes queries fail (no DoH option).
		blocks = append(blocks, dns.NewPlaintextBlock(nss, physIfs, ""))
		if params.chrome {
			tc = []dns.ProxyTestCase{{Client: dns.System, ExpectErr: true}, {Client: dns.User, ExpectErr: true}, {Client: dns.Chrome, ExpectErr: true}}
		} else if params.arc {
			tc = []dns.ProxyTestCase{{Client: dns.ARC, ExpectErr: true}}
		} else if params.crostini {
			tc = []dns.ProxyTestCase{{Client: dns.Crostini, ExpectErr: true}}
		}
	case dns.DoHAlwaysOn:
		// Verify blocking HTTPS causes queries to fail (no plaintext fallback).
		blocks = append(blocks, dns.NewDoHBlock(nss, physIfs))
		if params.chrome {
			tc = []dns.ProxyTestCase{{Client: dns.System, ExpectErr: true}, {Client: dns.User, ExpectErr: true}}
		} else if params.arc {
			tc = []dns.ProxyTestCase{{Client: dns.ARC, ExpectErr: true}}
		} else if params.crostini {
			tc = []dns.ProxyTestCase{{Client: dns.Crostini, ExpectErr: true}}
		}
	}

	for _, block := range blocks {
		if errs := block.Run(ctx, func(ctx context.Context) {
			if errs := dns.TestQueryDNSProxy(ctx, tc, a, cont, dns.NewQueryOptions()); len(errs) != 0 {
				s.Error("Failed DNS query check: ", errs)
			}
		}); len(errs) > 0 {
			s.Fatal("Failed to block DNS: ", errs)
		}
	}
}
