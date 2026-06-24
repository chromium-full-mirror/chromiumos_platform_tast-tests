// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/guestos"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
		Func:     DNSProxy,
		Desc:     "Ensure that DNS proxies are working correctly",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:release-health", "release-health_network"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      2 * time.Minute,
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
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_automatic",
			Val: dnsProxyTestParams{
				mode: dns.DoHAutomatic,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted",
		}, {
			Name: "arc_doh_always_on",
			Val: dnsProxyTestParams{
				mode: dns.DoHAlwaysOn,
				arc:  true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted",
		}, {
			Name: "crostini_doh_off",
			Val: dnsProxyTestParams{
				mode:     dns.DoHOff,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBookworm",
		}, {
			Name: "crostini_doh_automatic",
			Val: dnsProxyTestParams{
				mode:     dns.DoHAutomatic,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBookworm",
		}, {
			Name: "crostini_doh_always_on",
			Val: dnsProxyTestParams{
				mode:     dns.DoHAlwaysOn,
				crostini: true,
			},
			ExtraSoftwareDeps: []string{"vm_host", "dlc"},
			ExtraHardwareDeps: crostini.CrostiniStable,
			Fixture:           "crostiniBookworm",
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

	if params.arc {
		// Make sure that ARC gets the DNS proxy address before proceeding the
		// tests. Note the network created by dns.NewEnv() is IPv4-only.
		if err := arcnet.WaitForARCGetDNSProxyConfig(ctx, a, env.Router.VethOutName, true /*ipv4*/, false /*ipv6*/, 10*time.Second); err != nil {
			s.Fatal("Failed to wait for ARC to get the nameservers config: ", err)
		}

		// Verify DNS server can be pinged directly from ARC to make sure the
		// routing setup is ready for ARC.
		arcIfname, err := arcnet.GetARCInterfaceName(ctx, env.Router.VethOutName)
		if err != nil {
			s.Fatalf("Failed to get ARC interface name corresponding to %s: %v", env.Router.VethOutName, err)
		}
		if err := arcnet.ExpectPingSuccess(ctx, a, arcIfname, env.IPv4DNSAddr.String()); err != nil {
			s.Fatalf("Failed to verify DNS server %s reachability in ARC: %v", env.IPv4DNSAddr.String(), err)
		}
	}

	if params.crostini {
		// Make sure that routing setup for crostini is ready. Use a relatively
		// longer timeout here to make sure crostini have long enough time to finish
		// the network setup.
		if err := guestos.PingWithRetryAndTimeout(ctx, cont, env.IPv4DNSAddr.String(), 30*time.Second); err != nil {
			s.Fatalf("Failed to verify DNS server %s reachability in Crostini: %v", env.IPv4DNSAddr.String(), err)
		}
	}

	// By default, DNS query should work. Set AllowRetry to true in Chrome and
	// Crostini to be more permissive on the first attempt. We just restart
	// dnsproxy and switch the default network, it's expected that DNS has not
	// been ready on the system or there may be transient inconsistent state on
	// the system, and currently we don't have a good way to query that all things
	// are ready. For ARC, the readiness is already verified above.
	// TODO(jasongustaman): Verify resolv.conf at least before the query.
	var tcs []dns.ProxyTestCase
	if params.chrome {
		tcs = []dns.ProxyTestCase{
			{Client: dns.System, AllowRetry: true},
			{Client: dns.User, AllowRetry: true},
			{Client: dns.Chronos, AllowRetry: true},
		}
	} else if params.arc {
		tcs = []dns.ProxyTestCase{{Client: dns.ARC}}
	} else if params.crostini {
		tcs = []dns.ProxyTestCase{{Client: dns.Crostini, AllowRetry: true}}
	}
	for _, tc := range tcs {
		if err := tc.Run(ctx, nil /* chrome */, a, cont, dns.NewQueryOptions()); err != nil {
			s.Error("Failed DNS query check in the default setup: ", err)
		}
	}

	nss, err := dns.ProxyNamespaces(ctx)
	if err != nil {
		s.Fatal("Failed to get DNS proxy's network namespaces: ", err)
	}

	physIfs, err := physicalInterfaces(ctx)
	if err != nil {
		s.Fatal("Failed to get physical interfaces: ", err)
	}

	// Block plain-text or secure DNS through iptables.
	var blocks []*dns.Block
	switch params.mode {
	case dns.DoHAutomatic:
		// Confirm blocking plaintext still works (DoH preferred/used).
		blocks = append(blocks, dns.NewPlaintextBlock(nss, physIfs, "" /*dest*/, dns.ExampleDoHProviderHexString))
		// Verify blocking HTTPS also works (fallback).
		blocks = append(blocks, dns.NewDoHBlock(nss, physIfs))
		// Chrome isn't tested since it manages it's own DoH flow.
		// Allow retry for automatic mode. This is needed because Do53 fallback is only done after a DoH failure.
		// For this case, the failure happens on the proxy's DoH timeout which might be longer than the client's timeout.
		// Allow the client to retry the query. It is expected for the DoH server to be invalidated by then.
		if params.chrome {
			tcs = []dns.ProxyTestCase{{Client: dns.System, AllowRetry: true}, {Client: dns.User, AllowRetry: true}}
		} else if params.arc {
			tcs = []dns.ProxyTestCase{{Client: dns.ARC, AllowRetry: true}}
		} else if params.crostini {
			tcs = []dns.ProxyTestCase{{Client: dns.Crostini, AllowRetry: true}}
		}
	case dns.DoHOff:
		// Verify blocking plaintext causes queries fail (no DoH option).
		blocks = append(blocks, dns.NewPlaintextBlock(nss, physIfs, "" /*dest*/, "" /*excludeHexStr*/))
		if params.chrome {
			tcs = []dns.ProxyTestCase{{Client: dns.System, ExpectErr: true}, {Client: dns.User, ExpectErr: true}, {Client: dns.Chronos, ExpectErr: true}}
		} else if params.arc {
			tcs = []dns.ProxyTestCase{{Client: dns.ARC, ExpectErr: true}}
		} else if params.crostini {
			tcs = []dns.ProxyTestCase{{Client: dns.Crostini, ExpectErr: true}}
		}
	case dns.DoHAlwaysOn:
		// Verify blocking HTTPS causes queries to fail (no plaintext fallback).
		blocks = append(blocks, dns.NewDoHBlock(nss, physIfs))
		if params.chrome {
			tcs = []dns.ProxyTestCase{{Client: dns.System, ExpectErr: true}, {Client: dns.User, ExpectErr: true}}
		} else if params.arc {
			tcs = []dns.ProxyTestCase{{Client: dns.ARC, ExpectErr: true}}
		} else if params.crostini {
			tcs = []dns.ProxyTestCase{{Client: dns.Crostini, ExpectErr: true}}
		}
	}

	for _, block := range blocks {
		if errs := block.Run(ctx, func(ctx context.Context) {
			for _, tc := range tcs {
				if err := tc.Run(ctx, nil /* chrome */, a, cont, dns.NewQueryOptions()); err != nil {
					s.Errorf("Failed DNS query check in condition %s: %v", block, err)
				}
			}
		}); len(errs) > 0 {
			s.Fatalf("Failed to block DNS in condition %s: %v", block, errs)
		}
	}
}

// physicalInterfaces lists all available physical interfaces.
// This function returns the primary interface on multiplexed cell interface.
func physicalInterfaces(ctx context.Context) ([]string, error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create shill client")
	}

	devs, err := m.Devices(ctx)
	if err != nil {
		return nil, err
	}

	var ifnames []string
	for _, dev := range devs {
		p, err := dev.GetProperties(ctx)
		if err != nil {
			if dbusutil.IsDBusError(err, dbusutil.DBusErrorUnknownObject) {
				// This error is forgivable as a device may go down anytime.
				continue
			}
			return nil, err
		}
		t, err := p.GetString(shillconst.DevicePropertyType)
		if err != nil {
			testing.ContextLogf(ctx, "Error getting the device type %q: %v", dev, err)
			continue
		}
		ifProp := shillconst.DevicePropertyInterface
		if t == shillconst.TypeCellular {
			ifProp = shillconst.DevicePropertyCellularPrimaryMultiplexedInterface
		}
		if ifname, err := p.GetString(ifProp); err != nil {
			testing.ContextLogf(ctx, "Error getting the device interface %q: %v", dev, err)
			continue
		} else if ifname == "" {
			testing.ContextLogf(ctx, "Empty interface name for device %q", dev)
			continue
		} else {
			ifnames = append(ifnames, ifname)
		}
	}
	return ifnames, nil
}
