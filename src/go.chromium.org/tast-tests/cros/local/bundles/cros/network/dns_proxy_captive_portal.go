// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/captiveportalconsts"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type dnsProxyCaptivePortalTestParams struct {
	relog bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxyCaptivePortal,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify dns-proxy behaves correctly when shill detects a captive portal",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com", "garrick@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Params: []testing.Param{{
			Val: dnsProxyCaptivePortalTestParams{
				relog: false,
			},
			Fixture: "chromeLoggedIn.ehide",
			Timeout: 3 * time.Minute,
		}, {
			Name: "relog",
			Val: dnsProxyCaptivePortalTestParams{
				relog: true,
			},
			ExtraSoftwareDeps: []string{"arc"},
			Fixture:           "ehide",
			Timeout:           10 * time.Minute,
		}},
	})
}

func DNSProxyCaptivePortal(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	if err := m.EnablePortalDetection(ctx); err != nil {
		s.Fatal("Enable Portal Detection failed: ", err)
	}

	// Shill currently does not have a method to fully restart a portal
	// probe. Set both portal HTTPS URL and portal fallback HTTPS URLs to
	// accommodate captive portal check on first and subsequent probes.
	// The HTTPS URLs are necessary as we are using custom certificates for
	// virtualnet's network.
	url, err := m.GetAndSetProperty(ctx, shillconst.ManagerPropertyPortalHTTPSURL, captiveportalconsts.HTTPSPortalURL)
	if err != nil {
		s.Fatal("Failed to set portal https url: ", err)
	}
	defer func() {
		if err := m.SetProperty(cleanupCtx, shillconst.ManagerPropertyPortalHTTPSURL, url); err != nil {
			s.Error("Failed to revert portal https url: ", err)
		}
	}()
	fallbackURLs, err := m.GetAndSetProperty(ctx, shillconst.ManagerPropertyPortalFallbackHTTPSURLs, captiveportalconsts.HTTPSPortalURL)
	if err != nil {
		s.Fatal("Failed to set portal fallback https urls: ", err)
	}
	defer func() {
		if err := m.SetProperty(cleanupCtx, shillconst.ManagerPropertyPortalFallbackHTTPSURLs, fallbackURLs); err != nil {
			s.Error("Failed to revert portal fallback https urls: ", err)
		}
	}()

	httpsCerts := certs.New(certs.SSLCrtPath, certificate.TestCert3())
	cleanupCerts, err := httpsCerts.InstallTestCerts(ctx)
	if err != nil {
		s.Fatal("Failed to setup certificates: ", err)
	}
	defer cleanupCerts(cleanupCtx)

	relog := s.Param().(dnsProxyCaptivePortalTestParams).relog
	if relog {
		// If the test requires a relog virtualnet must be started
		// with Chrome logged out (b/300208832).
		// Restart UI to force the logged out state.
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}

	opts := virtualnet.EnvOptions{
		// No need to set Priority, since there is only 1 Ethernet in this test.
		NameSuffix:                 "",
		EnableDHCP:                 true,
		EnableDNS:                  true,
		RAServer:                   false,
		HTTPSServerResponseHandler: captiveportalconsts.NoContentHandler,
		HTTPServerResponseHandler:  captiveportalconsts.NoContentHandler,
		HTTPSCerts:                 httpsCerts,
	}
	pool := subnet.NewPool()
	veth, portalEnv, err := virtualnet.CreateRouterEnv(ctx, m, pool, opts)
	if err != nil {
		s.Fatal("Failed to create a portal env: ", err)
	}
	defer portalEnv.Cleanup(cleanupCtx)

	var a *arc.ARC
	if relog {
		// Start Chrome.
		cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)

		// Start ARC.
		a, err = arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(cleanupCtx)
	}

	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}
	if err := veth.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 30*time.Second); err != nil {
		s.Fatal("Service state is unexpected: ", err)
	}

	// Verify the system proxy is the current nameserver and name resolution works.
	// Give shill and dns-proxy sufficient time to respond to regaining connectivity.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.DigMatch(ctx, dns.DigProxyIPRE, true)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify system proxy: ", err)
	}
	if a != nil {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return dns.VerifyARCNameservers(ctx, a)
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Error("Failed to verify ARC: ", err)
		}
	}

	// Shill's captive portal detector works by probing various endpoints over HTTP and HTTPS.
	// So, first block these ports and then tell shill to rerun its portal detector, which should
	// fail, which should trigger shill to change the connection state from 'online' which in turn
	// should cause dns-proxy to disengage itself and tell shill to use the network's name servers.
	unblock := true
	defer func() {
		if unblock {
			network.UnblockShillPortalDetector(cleanupCtx)
		}
	}()

	if err := network.BlockShillPortalDetector(ctx); err != nil {
		s.Fatal("Failed to add rules to block portal detector: ", err)
	}
	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill")
	}
	if err := veth.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateNoConnectivity, 30*time.Second); err != nil {
		s.Fatal("Service state is unexpected: ", err)
	}

	// Verify the system proxy is not the current nameserver and name resolution works.
	// Give shill and dns-proxy sufficient time to respond to regaining connectivity.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.DigMatch(ctx, dns.DigProxyIPRE, false)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify switch over to underlying network's DNS: ", err)
	}

	if relog {
		// Re-start Chrome, emulate a logout and login.
		cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.EnableFeatures("EnableDnsProxy", "DnsProxyEnableDOH"))
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)

		// Start ARC.
		a, err = arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(cleanupCtx)

		// Verify the system proxy is still not the current nameserver and name resolution works.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return dns.DigMatch(ctx, dns.DigProxyIPRE, false)
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to verify switch over to underlying network's DNS: ", err)
		}
	}

	// Unblock the HTTP/S ports which should cause shill's portal detector to succeed, which will
	// move the connection state back to 'online' and subsequently reconnect dns-proxy.
	unblock = false
	if err := network.UnblockShillPortalDetector(ctx); err != nil {
		s.Fatal("Failed to remove rules to unblock portal detector: ", err)
	}
	if err := m.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}
	if err := veth.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 30*time.Second); err != nil {
		s.Fatal("Service state is unexpected: ", err)
	}

	// Verify the system proxy is the current nameserver and name resolution works.
	// Give shill and dns-proxy sufficient time to respond to regaining connectivity.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.DigMatch(ctx, dns.DigProxyIPRE, true)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify switchover to system proxy: ", err)
	}
	if a != nil {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return dns.VerifyARCNameservers(ctx, a)
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Error("Failed to verify ARC: ", err)
		}
	}
}
