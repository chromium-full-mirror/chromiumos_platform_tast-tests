// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type resolvConfCrashTestParams struct {
	shillCrash       bool
	controllerCrash  bool
	systemProxyCrash bool
	dnsProxyEnabled  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ResolvConfCrash,
		Desc:     "Verify resolv.conf is properly updated when there is a crash",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "shill_crash_proxy_disabled",
			Val: resolvConfCrashTestParams{
				shillCrash: true,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "shill_crash_proxy_enabled",
			Val: resolvConfCrashTestParams{
				shillCrash:      true,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "controller_crash_proxy_disabled",
			Val: resolvConfCrashTestParams{
				controllerCrash: true,
			},
			Fixture: "dnsProxyDisabled",
		}, {
			Name: "controller_crash_proxy_enabled",
			Val: resolvConfCrashTestParams{
				controllerCrash: true,
				dnsProxyEnabled: true,
			},
			Fixture: "chromeLoggedIn",
		}, {
			Name: "system_proxy_crash_proxy_enabled",
			Val: resolvConfCrashTestParams{
				systemProxyCrash: true,
				dnsProxyEnabled:  true,
			},
			Fixture: "chromeLoggedIn",
		}},
	})
}

// ResolvConfCrash tests /etc/resolv.conf content after crashes:
// 1. Add a base service.
// 2. Verify /etc/resolv.conf's content.
// 3. Overwrite /etc/resolv.conf to simulate stale data.
// 4. Simulate the crash for shill, controller, system proxy.
// 5. Verify /etc/resolv.conf's content.
// 6. Verify that /etc/resolv.conf is updated on shill service change.
func ResolvConfCrash(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
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

	params := s.Param().(resolvConfCrashTestParams)
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
			testing.ContextLog(cleanupCtx, "Failed to cleanup base service: ", err)
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

	// Trigger the crash.
	// Cleanup by restarting DNS proxy to avoid unexpected bad state after
	// the test. For instance, upon shill restart, DNS proxy needs to
	// re-connect to shill failing other tests on slow devices.
	defer func() {
		if err := dns.RestartDNSProxy(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to start DNS proxy: ", err)
		}
	}()
	if params.shillCrash {
		if err := testexec.CommandContext(ctx, "pkill", "-9", shill.JobName).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to restart shill: ", err)
		}
		// Disable portal detection again as the property is not persisted.
		// Shill might not be started yet here, poll until it is ready.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := m.DisablePortalDetection(ctx); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to disable portal detection: ", err)
		}
	} else if params.controllerCrash {
		// Stop DNS proxy's controller by stopping the oldest DNS proxy process.
		// As the controller starts other DNS proxy processes, it should be the oldest.
		if err := testexec.CommandContext(ctx, "pkill", "-9", "-x", "-o", dns.ProxyProcName).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to start DNS proxy's controller: ", err)
		}
		// This results in zombie child processes.
		defer func() {
			if err := testexec.CommandContext(cleanupCtx, "pkill", "-9", dns.ProxyProcName).Run(testexec.DumpLogOnError); err != nil {
				testing.ContextLog(cleanupCtx, "Failed to stop unwanted DNS proxy processes: ", err)
			}
		}()
	} else if params.systemProxyCrash {
		// Stop DNS proxy's system proxy.
		if err := testexec.CommandContext(ctx, "pkill", "-9", "-f", dns.ProxyProcName+" --t=sys").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to restart DNS proxy's system proxy: ", err)
		}
	}

	// Set up new network.
	newConfig := dns.Config{
		IPv4Nameservers:      []string{"2.2.2.1", "2.2.2.2"},
		IPv6Nameservers:      []string{"2222::2221", "2222::2222"},
		IPv4DomainSearchList: []string{"test2-1.com", "test2-2.com"},
	}
	newSvc, newR, err := dns.NewShillService(ctx, dns.EnvOptionsFromConfig(newConfig, "new" /* nameSuffix */, dns.HighPriority), pool)
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

	// Assert /etc/resolv.conf content after the new network is added.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.VerifyResolvConfContents(ctx, newConfig, params.dnsProxyEnabled)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check resolv.conf value after adding a new network: ", err)
	}
}
