// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxyUISettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensure that DoH settings in UI can be propagated to shill correctly",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      2 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

// DNSProxyUISettings tests that toggle the UI settings related to DoH, verify
// that the shill property is updated properly.
func DNSProxyUISettings(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set up virtualnet environment. Always-on mode needs the DoH server to be responding.
	env, err := dns.NewEnv(ctx, subnet.NewPool())
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	// Defer a function to reset the state to automatic. This is also part of the
	// test.
	defer func() {
		if _, err := dns.SetDoHMode(ctx, cr, tconn, dns.DoHAutomatic, "" /*dohProvider*/); err != nil {
			s.Fatal("Failed to set DNS-over-HTTPS mode to automatic: ", err)
		}
	}()

	if _, err := dns.SetDoHMode(ctx, cr, tconn, dns.DoHAlwaysOn, dns.ExampleDoHProvider); err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode to always-on: ", err)
	}

	if _, err := dns.SetDoHMode(ctx, cr, tconn, dns.DoHOff, "" /*dohProvider*/); err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode to off: ", err)
	}
}
