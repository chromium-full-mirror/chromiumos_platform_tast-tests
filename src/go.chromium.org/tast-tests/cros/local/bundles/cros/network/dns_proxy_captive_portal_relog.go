// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxyCaptivePortalRelog,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify dns-proxy behaves correctly when shill detects a captive portal and a re-login is done",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com", "garrick@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "arc", "no_kernel_upstream"},
		Timeout:      5 * time.Minute,
	})
}

func DNSProxyCaptivePortalRelog(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Start Chrome.
	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Start ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	// Ensure connectivity is available.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return testexec.CommandContext(ctx, "/bin/ping", "-c1", "-w1", "8.8.8.8").Run()
	}, &testing.PollOptions{Timeout: time.Second}); err != nil {
		s.Fatal("Failed to ping 8.8.8.8: ", err)
	}

	// Verify the system proxy is the current nameserver and name resolution works.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.DigMatch(ctx, dns.DigProxyIPRE, true)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify system proxy: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return verifyARCNameservers(ctx, a)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Error("Failed to verify ARC: ", err)
	}

	// Shill's captive portal detector works by probing various endpoints over HTTP and HTTPS.
	// So, first block these ports and then tell shill to rerun its portal detector, which should
	// fail, which should trigger shill to change the connection state from 'online' which in turn
	// should cause dns-proxy to disengage itself and tell shill to use the network's name servers.
	mgr, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager: ", err)
	}
	unblock := true
	defer func() {
		if unblock {
			network.UnblockShillPortalDetector(cleanupCtx)
		}
	}()
	if err := network.BlockShillPortalDetector(ctx); err != nil {
		s.Fatal("Failed to add rules to block portal detector: ", err)
	}
	if err := mgr.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}

	// Verify the system proxy is not the current nameserver and name resolution works.
	// Give shill and dns-proxy sufficient time to respond to the loss of connectivity.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.DigMatch(ctx, dns.DigProxyIPRE, false)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify switch over to Google DNS: ", err)
	}

	// Re-start Chrome, emulate a logout and login.
	cr, err = chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(),
		chrome.EnableFeatures("EnableDnsProxy", "DnsProxyEnableDOH"))
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
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify switch over to Google DNS: ", err)
	}

	// Unblock the HTTP/S ports which should cause shill's portal detector to succeed, which will
	// move the connection state back to 'online' and subsequently reconnect dns-proxy.
	unblock = false
	if err := network.UnblockShillPortalDetector(ctx); err != nil {
		s.Fatal("Failed to remove rules to unblock portal detector: ", err)
	}
	if err := mgr.RecheckPortal(ctx); err != nil {
		s.Fatal("Failed to invoke RecheckPortal on shill: ", err)
	}

	// Verify the system proxy is the current nameserver and name resolution works.
	// Give shill and dns-proxy sufficient time to respond to regaining connectivity.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dns.DigMatch(ctx, dns.DigProxyIPRE, true)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to verify switchover to system proxy: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return verifyARCNameservers(ctx, a)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Error("Failed to verify ARC: ", err)
	}
}

// verifyARCNameservers verify that ARC's nameservers contains DNS proxy address.
// The name servers are taken from MojoLinkProperties of ARC's `dumpsys wifi arc-networks`.
func verifyARCNameservers(ctx context.Context, a *arc.ARC) error {
	out, err := a.Command(ctx, "dumpsys", "wifi", "arc-networks").Output()
	if err != nil {
		return errors.Wrap(err, "failed to get ARC networks")
	}
	matches := dns.ARCNameserversRE.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return errors.New("empty name server")
	}
	for _, m := range matches {
		f := false
		// Index 0 contains the full match, start from index 1.
		for i := 1; i < len(m); i++ {
			if strings.Contains(m[i], dns.DNSProxyIPv4Prefix) {
				f = true
				break
			}
		}
		if !f {
			return errors.Errorf("invalid name server: %v", m)
		}
	}
	return nil
}
