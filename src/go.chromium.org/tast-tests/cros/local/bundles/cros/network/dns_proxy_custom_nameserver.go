// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"runtime"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DNSProxyCustomNameserver,
		Desc:     "Verify dns-proxy is elided when a custom nameserver is used",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:network", "network_platform"},
		SoftwareDeps: []string{"chrome", "arc", "no_kernel_upstream"},
		HardwareDeps: arc.ArcAppHwDeps,
		Data:         []string{digExecutable()},
		Timeout:      5 * time.Minute,
		Fixture:      "arcBooted",
	})
}

// DNSProxyCustomNameserver verifies that when a plaintext DNS query is made to a specific nameserver,
// explicitly overriding the network configuration, that dns-proxy is elided and the query goes directly to that server.
func DNSProxyCustomNameserver(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

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

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill client: ", err)
	}

	// Hide unused ethernet to avoid ARC's maximum number of ethernet
	// limitation.
	restoreEthernet, err := arcnet.HideUnusedEthernet(ctx, m)
	if err != nil {
		s.Fatal("Failed to hide unused ethernet: ", err)
	}
	defer restoreEthernet(cleanupCtx)

	// Disable the physical ethernet so that the only Ethernet service
	// active is the veth service created below.
	if enableFunc, err := m.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Unable to disable Ethernet: ", err)
	} else if enableFunc != nil {
		newCtx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime)
		defer cancel()
		defer enableFunc(ctx)
		ctx = newCtx
	}

	if enabled, err := m.IsEnabled(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Error calling IsEnabled: ", err)
	} else if enabled {
		s.Fatal("Ethernet is still enabled")
	}

	// Set up virtualnet environment.
	pool := subnet.NewPool()
	env, err := dns.NewEnv(ctx, pool)
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	// Create a new DNS server that acts as the custom name server.
	v4Subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv4 subnet: ", err)
	}
	v6Subnet, err := pool.AllocNextIPv6Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv6 subnet: ", err)
	}
	server, err := dns.NewServer(ctx, "c_server", v4Subnet, v6Subnet, env.Router, env.Certs)
	if err != nil {
		s.Fatal("Failed to create custom DNS nameserver: ", err)
	}
	defer server.Cleanup(cleanupCtx)

	addrs, err := server.Env.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get custom DNS nameserver's IP address: ", err)
	}

	// Make sure that ARC gets the DNS proxy address before proceeding the tests.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyARCNameservers(ctx, a)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for ARC to get the nameservers config: ", err)
	}

	// Ensure plaintext query.
	cleanup, err := dns.SetDoHModeViaShill(ctx, dns.DoHOff, "" /* dohProvider */)
	if err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode: ", err)
	}
	defer cleanup(cleanupCtx)

	p, err := dns.InstallDigInARC(ctx, a, s.DataPath(digExecutable()))
	if err != nil {
		s.Fatal("Failed to install dig in ARC: ", err)
	}

	// By default, host DNS queries work as-is.
	// TODO(b/232882301) - Add Crostini
	tcs := []dns.ProxyTestCase{
		{Client: dns.System},
		{Client: dns.User},
		{Client: dns.Chronos},
		{Client: dns.ARC},
	}
	for _, tc := range tcs {
		if err := tc.Run(ctx, nil /* chrome */, a, nil /* container */, dns.NewQueryOptions()); err != nil {
			s.Fatal("Failed initial DNS check: ", err)
		}
	}

	// Confirm that host queries to a different nameserver also work.
	opts := dns.NewQueryOptions()
	opts.Nameserver = addrs.IPv4Addr.To4().String()
	opts.ARCDigPath = p
	for _, tc := range tcs {
		if err := tc.Run(ctx, nil /* chrome */, a, nil /* container */, opts); err != nil {
			s.Fatal("Failed nameserver confirmation check: ", err)
		}
	}

	// Now block plaintext DNS traffic to that server and confirm failure to verify.
	nss, err := dns.ProxyNamespaces(ctx)
	if err != nil {
		s.Fatal("Failed to get DNS proxy's network namespaces: ", err)
	}
	if errs := dns.NewPlaintextBlock(nss, []string{env.Router.VethOutName}, opts.Nameserver, "" /*excludeHexStr*/).Run(ctx, func(ctx context.Context) {
		for i := 0; i < len(tcs); i++ {
			tcs[i].ExpectErr = true
		}
		opts.Domain = dns.RandDomain()
		for _, tc := range tcs {
			if err := tc.Run(ctx, nil /* chrome */, a, nil /* container */, opts); err != nil {
				s.Error("Failed nameserver verification: ", err)
			}
		}
	}); len(errs) > 0 {
		s.Fatal("Failed to block DNS to nameserver: ", errs)
	}
}

// digExecutable returns the dig executable name based on arch.
func digExecutable() string {
	switch runtime.GOARCH {
	case "arm":
		return "dig_arm"
	case "arm64":
		return "dig_arm64"
	default:
		return "dig_amd64"
	}
}
