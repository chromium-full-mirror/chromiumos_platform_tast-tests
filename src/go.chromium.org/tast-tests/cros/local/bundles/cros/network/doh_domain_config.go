// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/exp/slices"
)

type dohDomainConfigTestParams struct {
	mode dns.DoHMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     DoHDomainConfig,
		Desc:     "Ensure that DoH is used or bypassed correctly when DnsOverHttpsIncludedDomain and DnsOverHttpsExcludedDomain is set through policy",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Fixture:      fixture.ChromePolicyLoggedIn,
		Params: []testing.Param{{
			Name: "doh_off",
			Val: dohDomainConfigTestParams{
				mode: dns.DoHOff,
			},
		}, {
			Name: "doh_automatic",
			Val: dohDomainConfigTestParams{
				mode: dns.DoHAutomatic,
			},
		}, {
			Name: "doh_always_on",
			Val: dohDomainConfigTestParams{
				mode: dns.DoHAlwaysOn,
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DnsOverHttpsMode{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DnsOverHttpsTemplates{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DnsOverHttpsIncludedDomains{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DnsOverHttpsExcludedDomains{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// DoHDomainConfig tests DNS functionality when DnsOverHttpsIncludedDomains or DnsOverHttpsExcludedDomains is set.
// The test ensures that the correct DNS mechanism (DoH or Do53) is used.
func DoHDomainConfig(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	// Set up virtualnet environment.
	pool := subnet.NewPool()
	env, err := dns.NewEnv(ctx, pool)
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	params := s.Param().(dohDomainConfigTestParams)

	// Perform cleanup at the end of the test.
	defer func() {
		if err := policyutil.ResetChrome(cleanupCtx, fdms, cr); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to reset Chrome: ", err)
		}
	}()

	type dohDomainConfigTestCase struct {
		// Included and excluded domains set through policy.
		includedPolicy []string
		excludedPolicy []string
		// Domains that are expected to be included or excluded from using DoH.
		includedDomains []string
		excludedDomains []string
	}
	for _, dohTC := range []dohDomainConfigTestCase{
		{
			// Policy only contains included domains.
			includedPolicy:  []string{"test.com", "*.include.com"},
			includedDomains: []string{"test.com", "test.include.com"},
			excludedDomains: []string{"include.test.com", "include.com", "exclude.com"},
		},
		{
			// Policy only contains excluded domains.
			excludedPolicy:  []string{"test.com", "*.exclude.com"},
			includedDomains: []string{"include.text.com", "exclude.com", "include.com"},
			excludedDomains: []string{"test.com", "test.exclude.com"},
		},
		{
			// Policy contains included and excluded domains.
			includedPolicy:  []string{"test.com", "*.include.com", "include.exclude.include.com"},
			excludedPolicy:  []string{"exclude.include.com", "*.exclude.include.com"},
			includedDomains: []string{"test.com", "test.include.com", "include.exclude.include.com"},
			excludedDomains: []string{"exclude.include.com", "test.exclude.include.com", "exclude.com"},
		},
	} {
		// Perform cleanup before test.
		if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
			s.Fatal("Failed to reset Chrome: ", err)
		}

		// Update policies.
		provider := dns.ExampleDoHProvider
		if params.mode == dns.DoHOff {
			provider = ""
		}
		if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{
			&policy.DnsOverHttpsMode{Val: params.mode.String()},
			&policy.DnsOverHttpsTemplates{Val: provider},
			&policy.DnsOverHttpsIncludedDomains{Val: dohTC.includedPolicy},
			&policy.DnsOverHttpsExcludedDomains{Val: dohTC.excludedPolicy},
		}); err != nil {
			s.Fatal("Failed to update DoH included and excluded domains policies: ", err)
		}

		s.Log("DnsOverHttpsIncludedDomains is set to: ", dohTC.includedPolicy)
		s.Log("DnsOverHttpsExcludedDomains is set to: ", dohTC.excludedPolicy)

		// For automatic mode, we need to override the mapping of
		// DoH provider <-> nameserver that Chrome gave to shill.
		// This is necessary because we want to test the behavior of
		// the automatic upgrade.
		// Without overriding, devices with an arbitrary nameserver
		// without known DoH provider will only do Do53.
		// Cleanup is not run as it is already done through the policy
		// cleanup above.
		if params.mode == dns.DoHAutomatic {
			if _, err := dns.SetDoHModeViaShill(ctx, dns.DoHAutomatic, provider); err != nil {
				s.Fatal("Failed to set DNS-over-HTTPS mode: ", err)
			}
		}

		for _, tc := range []dns.ProxyTestCase{
			{Client: dns.System, AllowRetry: true},
			{Client: dns.User, AllowRetry: true},
			{Client: dns.Chronos, AllowRetry: true},
			{Client: dns.Chrome, AllowRetry: false},
		} {
			opts := dns.NewQueryOptions()
			for _, domain := range dohTC.includedDomains {
				opts.Domain = domain
				if err := tc.Run(ctx, cr, nil /* arc */, nil /* container */, opts); err != nil {
					s.Fatal("Failed to do DNS query: ", err)
				}
				if params.mode != dns.DoHOff && !slices.Contains(env.Server.DoHQueryLogs.Domains, domain) {
					s.Fatalf("Expect resolving %s to use DoH, got Do53", domain)
				}
				if params.mode == dns.DoHOff && slices.Contains(env.Server.DoHQueryLogs.Domains, domain) {
					s.Fatalf("Expect resolving %s to use Do53, got DoH", domain)
				}
			}
			for _, domain := range dohTC.excludedDomains {
				opts.Domain = domain
				if err := tc.Run(ctx, cr, nil /* arc */, nil /* container */, opts); err != nil {
					s.Fatal("Failed to do DNS query: ", err)
				}
				if slices.Contains(env.Server.DoHQueryLogs.Domains, domain) {
					s.Fatalf("Expect resolving %s to use Do53, got DoH", domain)
				}
			}
			env.Server.DoHQueryLogs.Clear()
		}
	}
}
