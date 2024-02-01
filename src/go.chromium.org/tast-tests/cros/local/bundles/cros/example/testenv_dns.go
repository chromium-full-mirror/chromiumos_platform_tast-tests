// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/testenv/middns"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TestenvDNS,
		Desc:         "Verify that middle-layer DNS redirects hosts using dnsmasq",
		Contacts:     []string{"cros-ufo-testing@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{},  // manual
	})
}

func TestenvDNS(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	d := middns.NewDNSServer()
	hostmap := []middns.FromTo{
		// Redirect all subdomains of example.com to 1.1.1.1.
		middns.FromTo{From: "example.com", To: "1.1.1.1"},
		// Redirect all subdomains of specific.example.com to 2.2.2.2.
		middns.FromTo{From: "specific.example.com", To: "2.2.2.2"},
		// Use "#" to bypass the DNS server and directly send query to the upstream nameservers that will return a real IP in production.
		middns.FromTo{From: "www.example.com", To: "#"},
	}

	if err := d.Start(ctx, hostmap); err != nil {
		s.Fatal("Failed to start local DNS server: ", err)
	}
	defer d.Stop(cleanupCtx)

	// Verify that the DNS server redirects all subdomains of example.com, specific domains
	// and resolution bypass as configured.
	// Example:
	//	example.com will go to 1.1.1.1, and other subdomains (a.example.com) to 1.1.1.1 as well.
	//	specific.example.com will go to 2.2.2.2
	//	www.example.com will bypass the DNS server and go to public IP.
	for _, tc := range []struct {
		host        string
		ip          string
		shouldEqual bool
	}{
		{"example.com", "1.1.1.1", true},
		{"a.example.com", "1.1.1.1", true},
		{"specific.example.com", "2.2.2.2", true},
		{"www.example.com", "1.1.1.1", false},
	} {
		var m middns.QueryMatcher
		if tc.shouldEqual {
			m = middns.AllOf(tc.host, tc.ip, d.IP)
		} else {
			m = middns.OnlyHostNotServer(tc.host, tc.ip, d.IP)
		}
		if err := middns.VerifyQuery(ctx, tc.host, m, 15*time.Second); err != nil {
			s.Fatalf("Failed to redirect for host: %v, %v", tc.host, err)
		}
	}

	// Stop the DNS server
	if err := d.Stop(cleanupCtx); err != nil {
		s.Fatal("Failed to stop local DNS server: ", err)
	}

	// Verify that the example.com should not go to 1.1.1.1 and talk to the DNS server once it has stopped.
	if err := middns.VerifyQuery(ctx,
		"example.com", middns.NoneOf("example.com", "1.1.1.1", d.IP), 15*time.Second); err != nil {
		s.Fatalf("Failed to stop redirect for host: %v, %v", "example.com", err)
	}
}
