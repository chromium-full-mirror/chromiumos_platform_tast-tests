// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DNSProxyUISettings,
		Desc:     "Ensure that DoH settings in UI can be propagated to shill correctly",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
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

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	defer cancel()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	dumpUI := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "dnsui" /*prefix*/)
	s.AttachErrorHandlers(dumpUI, dumpUI)

	// Start screen recording, to help with debugging errors.
	recorder, err := uiauto.NewScreenRecorder(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to create screen recorder: ", err)
	}
	if recorder.Start(ctx, tconn); err != nil {
		s.Fatal("Failed to start screen recorder: ", err)
	}
	defer recorder.StopAndSaveOnError(cleanupCtx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)

	// Set up virtualnet environment. Always-on mode needs the DoH server to be responding.
	env, err := dns.NewEnv(ctx, subnet.NewPool())
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	// Reset to off at the end of the test.
	defer func() {
		if err := dns.SetDoHModeViaUI(ctx, cr, tconn, dns.DoHOff, "" /*dohProvider*/); err != nil {
			s.Log("Failed to set DNS-over-HTTPS mode to off: ", err)
		}
	}()

	if err := dns.SetDoHModeViaUI(ctx, cr, tconn, dns.DoHOff, "" /*dohProvider*/); err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode to off: ", err)
	}

	if err := dns.SetDoHModeViaUI(ctx, cr, tconn, dns.DoHAutomatic, "" /*dohProvider*/); err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode to automatic: ", err)
	}

	if err := dns.SetDoHModeViaUI(ctx, cr, tconn, dns.DoHAlwaysOn, dns.ExampleDoHProvider); err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode to always-on: ", err)
	}
}
