// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type resolvConfLowerPriorityConfigUpdateTestParams struct {
	dnsProxyEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ResolvConfLowerPriorityConfigUpdate,
		Desc:     "Verify resolv.conf is not updated on lower priority network configuration update",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name:    "proxy_disabled",
			Val:     resolvConfLowerPriorityConfigUpdateTestParams{},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "proxy_enabled",
			Val: resolvConfLowerPriorityConfigUpdateTestParams{
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}},
	})
}

// ResolvConfLowerPriorityConfigUpdate tests /etc/resolv.conf content after a lower priority network config update:
// 1. Add a base service.
// 2. Add a new service with lower priority.
// 3. Verify /etc/resolv.conf's content to be of the higher priority's network.
// 4. Update the lower priority service's network config using StaticIPConfig.
// 5. Verify /etc/resolv.conf's content to not be updated.
func ResolvConfLowerPriorityConfigUpdate(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

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

	params := s.Param().(resolvConfLowerPriorityConfigUpdateTestParams)
	pool := subnet.NewPool()

	// Set up base network.
	baseConfig := dns.Config{
		IPv4Nameservers:      []string{"1.1.1.1", "1.1.1.2"},
		IPv6Nameservers:      []string{"1111::1111", "1111::1112"},
		IPv4DomainSearchList: []string{"test1-1.com", "test1-2.com"},
	}
	baseSvc, baseR, err := dns.NewShillService(ctx, dns.EnvOptionsFromConfig(baseConfig, "base" /* nameSuffix */, dns.HighPriority), pool)
	if err != nil {
		s.Fatal("Failed to set up base network: ", err)
	}
	defer func() {
		if err := baseR.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to cleanup router: ", err)
		}
	}()
	if err := m.WaitForDefaultService(ctx, baseSvc); err != nil {
		s.Fatal("Failed to wait for the base network to become the default network: ", err)
	}

	// Set up lower priority network.
	lowConfig := dns.Config{
		IPv4Nameservers:      []string{"2.2.2.1", "2.2.2.2"},
		IPv6Nameservers:      []string{"2222::2221", "2222::2222"},
		IPv4DomainSearchList: []string{"test2-1.com", "test2-2.com"},
	}
	lowSvc, lowR, err := dns.NewShillService(ctx, dns.EnvOptionsFromConfig(lowConfig, "low" /* nameSuffix */, dns.LowPriority), pool)
	if err != nil {
		s.Fatal("Failed to set up lower priority network: ", err)
	}
	defer func() {
		if err := lowR.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to cleanup router: ", err)
		}
	}()

	// Assert that /etc/resolv.conf is correct.
	// The poll is necessary as the IPv6 nameservers might not be pushed yet.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, baseConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value: ", err)
	}

	// Update config for the base network using StaticIPConfig.
	newConfig := dns.Config{
		IPv4Nameservers:      []string{"3.3.3.1", "3.3.3.2"},
		IPv6Nameservers:      []string{"3333::3331", "3333::3332"},
		IPv4DomainSearchList: []string{"test3-1.com", "test3-2.com"},
	}
	svcStaticIPConfig := map[string]interface{}{
		shillconst.IPConfigPropertySearchDomains: newConfig.IPv4DomainSearchList,
		shillconst.IPConfigPropertyNameServers:   append(newConfig.IPv4Nameservers, newConfig.IPv6Nameservers...),
	}
	testing.ContextLogf(ctx, "Configuring %v on the lower priority interface", svcStaticIPConfig)
	if err := lowSvc.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, svcStaticIPConfig); err != nil {
		s.Fatal("Failed to configure StaticIPConfig property on the test service: ", err)
	}
	defer func(ctx context.Context) {
		// Reset StaticIPConfig before removing the test interfaces, to avoid
		// installing this address on the physical interfaces. See
		// b/239753191#comment8 for a racing case.
		if err := lowSvc.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, map[string]interface{}{}); err != nil {
			testing.ContextLog(ctx, "Failed to reset StaticIPConfig property on the test service: ", err)
		}
	}(cleanupCtx)

	// Assert /etc/resolv.conf content to not be changed after the lower priority network is updated.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, baseConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value after lower priority network update: ", err)
	}
}
