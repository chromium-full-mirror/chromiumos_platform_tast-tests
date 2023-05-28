// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/demomode/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify state about the core demo mode Chrome Apps",
		Contacts:     []string{"cros-demo-mode-eng@google.com", "jacksontadie@google.com"},
		// Chrome OS Server Projects > Enterprise Management > Demo Mode
		BugComponent: "b:812312",
		Fixture:      fixture.PostDemoModeOOBE,
		Attr:         []string{"group:mainline", "informational"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "tpm2"},
	})
}

// ChromeApps asserts state about the core auto-launched Demo Mode Chrome Apps:
// - Attract Loop: An animated screensaver that loops while a device is idle.
// - Highlights App: A windowed interactive application that showcases ChromeOS features.
func ChromeApps(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.KeepEnrollment(),
		// Disable DemoModeSWA feature as this replaces the Chrome Apps.
		chrome.DisableFeatures("DemoModeSWA"),
		// Force devtools on regardless of policy (devtools is disabled in
		// Demo Mode policy) to support connecting to the test API extension.
		chrome.ExtraArgs("--force-devtools-available"))
	if err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}
	clearUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer cr.Close(clearUpCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(clearUpCtx, s.OutDir(), s.HasError, tconn)

	// Have such a long timeout because the Attract Loop takes a while to load
	// for the first session after setup (app is cached for subsequent sessions).
	ui := uiauto.New(tconn).WithTimeout(100 * time.Second)

	s.Log("Waiting for Highlights App")
	highlightsWindow := nodewith.Name("Google Retail Chromebook").First()
	if err := ui.WaitUntilExists(highlightsWindow)(ctx); err != nil {
		s.Fatal("Failed to wait until Highlights App exists: ", err)
	}
}
