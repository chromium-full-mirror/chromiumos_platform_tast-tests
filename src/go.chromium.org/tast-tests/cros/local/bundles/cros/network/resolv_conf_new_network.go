// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type resolvConfNewNetworkTestParams struct {
	priority        int
	vpn             bool
	dnsProxyEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ResolvConfNewNetwork,
		Desc:     "Verify resolv.conf is properly updated on network changes",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "low_priority_proxy_disabled",
			Val: resolvConfNewNetworkTestParams{
				priority: dns.LowPriority,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "low_priority_proxy_enabled",
			Val: resolvConfNewNetworkTestParams{
				priority:        dns.LowPriority,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "high_priority_proxy_disabled",
			Val: resolvConfNewNetworkTestParams{
				priority: dns.HighPriority,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "high_priority_proxy_enabled",
			Val: resolvConfNewNetworkTestParams{
				priority:        dns.HighPriority,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "vpn_proxy_disabled",
			Val: resolvConfNewNetworkTestParams{
				priority: dns.HighPriority,
				vpn:      true,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "vpn_proxy_enabled",
			Val: resolvConfNewNetworkTestParams{
				priority:        dns.HighPriority,
				vpn:             true,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}},
	})
}

// ResolvConfNewNetwork tests /etc/resolv.conf content after a new network is added:
// 1. Create the base service.
// 2. Verify /etc/resolv.conf's content.
// 3. Add a new service.
// 4. Verify /etc/resolv.conf's content depending on the priority of the new service.
// 5. Remove the new service.
// 6. Verify /etc/resolv.conf's content to be the base service.
func ResolvConfNewNetwork(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Shill-related setup.
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}
	testing.ContextLog(ctx, "Disabling portal detection")
	if err := m.DisablePortalDetection(ctx); err != nil {
		s.Fatal("Failed to disable portal detection: ", err)
	}
	defer func() {
		if err := m.EnablePortalDetection(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to re-enable portal detection: ", err)
		}
	}()

	params := s.Param().(resolvConfNewNetworkTestParams)
	pool := subnet.NewPool()

	// Set up base network.
	baseConfig := dns.Config{
		IPv4Nameservers:      []string{"1.1.1.1", "1.1.1.2"},
		IPv6Nameservers:      []string{"1111::1111", "1111::1112"},
		IPv4DomainSearchList: []string{"test1-1.com", "test1-2.com"},
	}
	baseSvc, baseR, err := dns.NewShillService(ctx, dns.EnvOptionsFromConfig(baseConfig, "base" /* nameSuffix */, dns.BasePriority), pool)
	if err != nil {
		s.Fatal("Failed to set up base network: ", err)
	}
	defer func() {
		if err := baseR.Cleanup(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to cleanup the base service: ", err)
		}
	}()
	if err := m.WaitForDefaultService(ctx, baseSvc); err != nil {
		s.Fatal("Failed to wait for the base network to become the default network: ", err)
	}

	// Assert that /etc/resolv.conf is correct.
	// The poll is necessary as the IPv6 nameservers might not be pushed yet.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, baseConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value: ", err)
	}

	var (
		newConfig dns.Config
		conn      *vpn.Connection
		newR      *env.Env
		newSvc    *shill.Service
	)
	cleanupVPN := func(ctx context.Context) {
		if conn == nil {
			return
		}
		defer func() {
			// Don't retry cleanup on failure.
			conn = nil
		}()
		if err := conn.Cleanup(ctx); err != nil {
			s.Fatal("Failed to cleanup the VPN connection: ", err)
		}
	}
	cleanupNewR := func(ctx context.Context) {
		if newR == nil {
			return
		}
		defer func() {
			// Don't retry cleanup on failure.
			newR = nil
		}()
		if err := newR.Cleanup(ctx); err != nil {
			s.Fatal("Failed to cleanup the new network: ", err)
		}
	}

	// Set up new network.
	if params.vpn {
		searchDomain := []string{"test2-1.com", "test2-2.com"}
		conn, err = vpn.StartConnection(ctx, baseR, vpn.TypeIKEv2, vpn.WithSearchDomains(searchDomain))
		if err != nil {
			s.Fatal("Failed to connect vpn: ", err)
		}
		defer cleanupVPN(cleanupCtx)
		newConfig = dns.Config{
			IPv4Nameservers:      []string{conn.Server.OverlayIPv4}, // tast test IKEv2 server pushes server overlay address as DNS by default.
			IPv4DomainSearchList: searchDomain,
		}
	} else {
		newConfig = dns.Config{
			IPv4Nameservers:      []string{"2.2.2.1", "2.2.2.2"},
			IPv6Nameservers:      []string{"2222::2221", "2222::2222"},
			IPv4DomainSearchList: []string{"test2-1.com", "test2-2.com"},
		}
		newSvc, newR, err = dns.NewShillService(ctx, dns.EnvOptionsFromConfig(newConfig, "new" /* nameSuffix */, params.priority), pool)
		if err != nil {
			s.Fatal("Failed to set up a new network: ", err)
		}
		defer cleanupNewR(cleanupCtx)
		if params.priority == dns.HighPriority {
			if err := m.WaitForDefaultService(ctx, newSvc); err != nil {
				s.Fatal("Failed to wait for the new network to become the default network: ", err)
			}
		}
	}

	// Assert /etc/resolv.conf content after the new network is added.
	expectedConfig := baseConfig
	if params.priority == dns.HighPriority {
		expectedConfig = newConfig
	}
	if params.vpn && params.dnsProxyEnabled {
		// With DNS proxy and VPN enabled, system proxy will update /etc/resolv.conf
		// based on IP address content of the default physical network.
		expectedConfig = dns.Config{
			IPv4Nameservers:      baseConfig.IPv4Nameservers,
			IPv6Nameservers:      baseConfig.IPv6Nameservers,
			IPv4DomainSearchList: newConfig.IPv4DomainSearchList}
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, expectedConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value after adding a new network: ", err)
	}

	// Cleanup the new service.
	cleanupVPN(ctx)
	cleanupNewR(ctx)
	if err := m.WaitForDefaultService(ctx, baseSvc); err != nil {
		s.Fatal("Failed to wait for the base network to become the default network: ", err)
	}

	// Assert /etc/resolv.conf content after the new network is removed.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, baseConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value after removing the new network: ", err)
	}
}
