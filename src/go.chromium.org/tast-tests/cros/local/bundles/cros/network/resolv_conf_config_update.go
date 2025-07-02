// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/network/dumputil"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type resolvConfConfigUpdateTestParams struct {
	ipv4            bool
	ipv6            bool
	dnsProxyEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ResolvConfConfigUpdate,
		Desc:     "Verify resolv.conf is properly updated on network configuration update",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Params: []testing.Param{{
			Name: "ipv4_only_proxy_disabled",
			Val: resolvConfConfigUpdateTestParams{
				ipv4: true,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "ipv4_only_proxy_enabled",
			Val: resolvConfConfigUpdateTestParams{
				ipv4:            true,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "ipv6_only_proxy_disabled",
			Val: resolvConfConfigUpdateTestParams{
				ipv6: true,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "ipv6_only_proxy_enabled",
			Val: resolvConfConfigUpdateTestParams{
				ipv6:            true,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "dual_stack_proxy_disabled",
			Val: resolvConfConfigUpdateTestParams{
				ipv4: true,
				ipv6: true,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "dual_stack_proxy_enabled",
			Val: resolvConfConfigUpdateTestParams{
				ipv4:            true,
				ipv6:            true,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}},
	})
}

// ResolvConfConfigUpdate tests /etc/resolv.conf content when there is a config update:
// 1. Add a base service.
// 2. Verify /etc/resolv.conf's content.
// 3. Update the service's config using shill's StaticIPConfig.
// 4. Verify /etc/resolv.conf's content.
func ResolvConfConfigUpdate(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Dump network info on failure.
	errorHandler := dumputil.CreateErrorHandler(cleanupCtx)
	s.AttachErrorHandlers(errorHandler, errorHandler)

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewResetVirtualnetHook(),
		testhooks.NewDisablePortalDetectionHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	params := s.Param().(resolvConfConfigUpdateTestParams)
	pool := subnet.NewPool()

	// Set up base network.
	var baseConfig dns.Config
	if params.ipv4 {
		baseConfig.IPv4Nameservers = []string{"1.1.1.1", "1.1.1.2"}
		baseConfig.IPv4DomainSearchList = []string{"test1-1.com", "test1-2.com"}
	}
	if params.ipv6 {
		baseConfig.IPv6Nameservers = []string{"1111::1111", "1111::1112"}
	}
	svc, r, err := dns.NewShillService(ctx, dns.EnvOptionsFromConfig(baseConfig, "base", dns.BasePriority), pool)
	if err != nil {
		s.Fatal("Failed to set up base network: ", err)
	}
	defer func() {
		if err := r.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to cleanup router: ", err)
		}
	}()
	if err := m.WaitForDefaultService(ctx, svc); err != nil {
		s.Fatal("Failed to wait for the base network to become the default network: ", err)
	}

	// Assert that /etc/resolv.conf is correct.
	// The poll is necessary as the IPv6 nameservers might not be pushed yet.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, baseConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value: ", err)
	}

	// TODO(b/298565265): Network update through StaticIPConfig is currently only triggered when IPv4 is available.
	if !params.ipv4 {
		return
	}

	// Update config for the created network using StaticIPConfig.
	newConfig := dns.Config{
		IPv4Nameservers:      []string{"2.2.2.1", "2.2.2.2"},
		IPv4DomainSearchList: []string{"test2-1.com", "test2-2.com"},
	}
	staticIPConfigNameServers := newConfig.IPv4Nameservers
	if params.ipv6 {
		newConfig.IPv6Nameservers = []string{"2222::2221", "2222::2222"}
		// StaticIPConfig does not differentiate between IPv4 and IPv6. Put IPv6 at
		// first because in the current implementation of shill, the ordering of
		// name servers in StaticIPConfig will be honored, but it's not convenient
		// to represent this ordering in the dns package in tast where IPv6 is
		// preferred, so we put the IPv6 ones at first here to match their ordering.
		staticIPConfigNameServers = append(newConfig.IPv6Nameservers, staticIPConfigNameServers...)
	}
	svcStaticIPConfig := map[string]interface{}{
		shillconst.IPConfigPropertySearchDomains: newConfig.IPv4DomainSearchList,
		shillconst.IPConfigPropertyNameServers:   staticIPConfigNameServers,
	}
	testing.ContextLogf(ctx, "Configuring %v on the test interface", svcStaticIPConfig)
	if err := svc.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, svcStaticIPConfig); err != nil {
		s.Fatal("Failed to configure StaticIPConfig property on the test service: ", err)
	}
	defer func(ctx context.Context) {
		// Reset StaticIPConfig before removing the test interfaces, to avoid
		// installing this address on the physical interfaces. See
		// b/239753191#comment8 for a racing case.
		if err := svc.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, map[string]interface{}{}); err != nil {
			testing.ContextLog(ctx, "Failed to reset StaticIPConfig property on the test service: ", err)
		}
	}(cleanupCtx)

	// Assert /etc/resolv.conf content after the network is updated.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, newConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value after applying the StaticIPConfig: ", err)
	}
}
