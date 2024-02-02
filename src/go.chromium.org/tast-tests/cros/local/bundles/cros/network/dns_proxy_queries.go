// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxyQueries,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensure that DNS queries are successful",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:cq-medium", "group:network", "network_cq"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      1 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

// DNSProxyQueries tests DNS queries through DNS proxy.
// There are 2 parts to this test:
// 1. Ensuring that DNS queries through Do53 are successful.
// 2. Ensuring that DNS queries through DoH are successful.
// The test is expected to run in networking medium CQ alongside other tests.
// As there is a time constraint of 30s for the networking medium CQ runs, this
// test is expected to run in much less time than 30s.
// Avoid increasing the run time of the test.
func DNSProxyQueries(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Set up virtualnet environment.
	env, err := dns.NewEnv(ctx, subnet.NewPool())
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	// Wait for DNS proxy to be ready after the network change.
	// This can take longer than 1s as DNS proxy is restarted when creating
	// the new network environment to be able to read updated certificates.
	// GoBigSleepLint: The sleep is necessary as there is currently no
	// way of querying DNS proxy's state.
	// TODO(b/281778714): Add a way to query DNS proxy's state.
	testing.Sleep(ctx, 1*time.Second)

	// Allow retry as the sleep might not be enough on rare cases of slow
	// DNS proxy startup. Keep the sleep as retries take more time.
	tc := []dns.ProxyTestCase{
		{Client: dns.System, AllowRetry: true},
		{Client: dns.User, AllowRetry: true},
		{Client: dns.Chrome, AllowRetry: true},
	}

	s.Log("Querying through Do53")
	if errs := dns.TestQueryDNSProxy(ctx, tc, nil /* arc */, nil /* container */, dns.NewQueryOptions()); len(errs) != 0 {
		for _, err := range errs {
			s.Error("Failed DNS query check: ", err)
		}
	}

	s.Log("Setting DoH provider")
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill client: ", err)
	}
	if err := m.SetDNSProxyDOHProviders(ctx, map[string]interface{}{dns.ExampleDoHProvider: ""}); err != nil {
		s.Fatal("Failed to set dns-proxy DoH providers: ", err)
	}
	defer func() {
		if err := m.SetDNSProxyDOHProviders(cleanupCtx, map[string]interface{}{}); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to set reset dns-proxy DoH providers: ", err)
		}
	}()

	// DoH providers are propagated to DNS proxy from shill call above.
	// The propagation is expected to happen very quickly (<100 ms).
	// In order to avoid flakiness on edge cases, add sleep. The test is
	// expected to pass even without the sleep on the normal case.
	// GoBigSleepLint: The sleep is necessary as there is currently no
	// way of querying DNS proxy's state.
	// TODO(b/281778714): Add a way to query DNS proxy's state.
	testing.Sleep(ctx, 1*time.Second)

	s.Log("Querying through DoH")
	if errs := dns.TestQueryDNSProxy(ctx, tc, nil /* arc */, nil /* container */, dns.NewQueryOptions()); len(errs) != 0 {
		for _, err := range errs {
			s.Error("Failed DNS query check: ", err)
		}
	}
}
