// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/testenv/middns"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyMidDNSARC,
		Desc:         "Verify that a mid DNS server redirects hosts for ARC VM",
		Contacts:     []string{"cros-ufo-testing@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "b:1528139", // ChromeOS > EngProd > Software > Trust & Safety > UFO Testing
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome", "gaia", "arc"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
	})
}

func VerifyMidDNSARC(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Start Chrome with ARC enabled
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start chrome with ARC enabled: ", err)
	}
	defer cr.Close(cleanupCtx)

	s.Log("Starting DNS server to redirect traffic for ARC")
	d := middns.NewDNSServer()
	hostmap := map[string]string{
		"example.com": "1.1.1.1",
	}
	if err := d.Start(ctx, hostmap); err != nil {
		s.Fatal("Failed to start local DNS server: ", err)
	}
	defer d.Close(cleanupCtx)

	// Check that the example.com is properly redirected to 1.1.1.1 by the DNS server using `dig` command.
	if err := middns.VerifyQuery(ctx,
		"example.com", middns.ByHostAndIP("example.com", "1.1.1.1"), 15*time.Second); err != nil {
		s.Fatal("DNS redirection verification failed: ", err)
	}
	s.Log("Host redirection succeeded on cros")

	// Wait for ARC to start and ADB to be setup, which would take a bit long.
	if err := optin.PerformWithRetry(ctx, cr, 2 /*maxAttempts*/); err != nil {
		s.Fatal("Failed to optin to Play Store")
	}
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), 4*time.Minute, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	// Check if Android respects the host redirection that the mid DNS server set above.
	// by getprop eg, [vendor.arc.net.host_dns]: [dns_ip[,upstream_dns_ip]]
	val, err := a.GetProp(ctx, "vendor.arc.net.host_dns")
	if err != nil {
		s.Fatal("Failed to run getprop: ", err)
	}
	if !strings.HasPrefix(val, d.IP) {
		s.Fatalf("Failed to find DNS IP (%v) in getprop: %v", d.IP, err)
	}
	// by ping
	out, err := arc.BootstrapCommand(ctx, "/system/bin/ping", "-c1", "-W1", "example.com").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run ping: ", err)
	}
	re := regexp.MustCompile(fmt.Sprintf(`PING:\s+%s\s\(%s\)\b`, "example.com", "1.1.1.1"))
	if re.MatchString(string(out)) {
		s.Fatal("host not found in ping")
	}
	s.Log("Host redirection succeeded on ARC")
}
