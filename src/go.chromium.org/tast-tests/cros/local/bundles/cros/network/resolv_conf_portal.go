// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ResolvConfPortal,
		Desc:     "Verify resolv.conf is properly updated on network changes of non-online networks",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Params: []testing.Param{{
			Name:    "proxy_enabled",
			Fixture: "chromeLoggedIn",
		}},
	})
}

// ResolvConfPortal tests /etc/resolv.conf content after a new network is added (without any online network):
// 1. Block portal detector such that the network is not online.
// 2. Create the base service.
// 3. Verify /etc/resolv.conf's content.
// 4. Add a new service with a higher priority.
// 5. Verify /etc/resolv.conf's content to be updated with the new service.
func ResolvConfPortal(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewResetVirtualnetHook(),
		testhooks.NewEnablePortalDetectionHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	// Shill setup.
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}
	if err := network.BlockShillPortalDetector(ctx); err != nil {
		s.Fatal("Failed to block portal detector: ", err)
	}
	defer network.UnblockShillPortalDetector(cleanupCtx)
	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}

	pool := subnet.NewPool()

	// Set up base network.
	baseConfig := dns.Config{
		IPv4Nameservers:      []string{"1.1.1.1", "1.1.1.2"},
		IPv6Nameservers:      []string{"1111::1111", "1111::1112"},
		IPv4DomainSearchList: []string{"test1-1.com", "test1-2.com"},
	}
	baseSvc, baseR, err := virtualnet.CreateRouterEnv(ctx, m, pool, virtualnet.EnvOptions{
		Priority:             dns.BasePriority,
		NameSuffix:           "base",
		IPv4DNSServers:       baseConfig.IPv4Nameservers,
		IPv6DNSServers:       baseConfig.IPv6Nameservers,
		IPv4DomainSearchList: baseConfig.IPv4DomainSearchList,
		EnableDHCP:           true,
		RAServer:             true,
	})
	if err != nil {
		s.Fatal("Failed to set up base network: ", err)
	}
	defer func() {
		if err := baseR.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to cleanup base service: ", err)
		}
	}()
	if err := m.WaitForDefaultService(ctx, baseSvc); err != nil {
		s.Fatal("Failed to wait for the base network to become the default network: ", err)
	}
	if err := dns.WaitForNameServers(ctx, baseSvc, append(baseConfig.IPv4Nameservers, baseConfig.IPv6Nameservers...)); err != nil {
		s.Fatal("Failed to wait for the base network to properly have all nameservers: ", err)
	}

	// Assert that /etc/resolv.conf is populated with base config.
	// DNS proxy should not be used on non-online networks.
	// Give shill and dns-proxy sufficient time to respond to the loss of connectivity.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, baseConfig, false /* dnsProxyEnabled */)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value: ", err)
	}

	// Set up new network.
	// Verify that /etc/resolv.conf is updated properly for non-online network changes.
	newConfig := dns.Config{
		IPv4Nameservers:      []string{"2.2.2.1", "2.2.2.2"},
		IPv6Nameservers:      []string{"2222::2221", "2222::2222"},
		IPv4DomainSearchList: []string{"test2-1.com", "test2-2.com"},
	}
	newSvc, newR, err := virtualnet.CreateRouterEnv(ctx, m, pool, virtualnet.EnvOptions{
		Priority:             dns.HighPriority,
		NameSuffix:           "new",
		IPv4DNSServers:       newConfig.IPv4Nameservers,
		IPv6DNSServers:       newConfig.IPv6Nameservers,
		IPv4DomainSearchList: newConfig.IPv4DomainSearchList,
		EnableDHCP:           true,
		RAServer:             true,
	})
	if err != nil {
		s.Fatal("Failed to set up a new network: ", err)
	}
	defer func() {
		if err := newR.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to cleanup the new service: ", err)
		}
	}()
	if err := m.WaitForDefaultService(ctx, newSvc); err != nil {
		s.Fatal("Failed to wait for the new network to become the default network: ", err)
	}
	if err := dns.WaitForNameServers(ctx, newSvc, append(newConfig.IPv4Nameservers, newConfig.IPv6Nameservers...)); err != nil {
		s.Fatal("Failed to wait for the new network to properly have all nameservers: ", err)
	}

	// Assert /etc/resolv.conf content after the new network is added.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, newConfig, false /* dnsProxyEnabled */)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value after adding a new network: ", err)
	}
}
