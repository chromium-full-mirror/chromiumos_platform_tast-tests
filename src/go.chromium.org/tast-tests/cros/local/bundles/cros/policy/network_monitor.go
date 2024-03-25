// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy/mitmproxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkMonitor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that network requests go through local proxy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"yanghenry@google.com",
			"donnadionne@google.com",
		},
		BugComponent: "b:1129862", // ChromeOS > Privacy > DPChromeOS > DPChromeOS Engineering
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Fixture:      fixture.ChromeLoggedIn,
		Attr:         []string{"group:hw_agnostic"},
	})
}

func NetworkMonitor(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	mp, err := mitmproxy.New(ctx)
	if err != nil {
		s.Fatal("Failed to create MitmProxy: ", err)
	}

	if err := cr.LaunchAndApplyProxy(ctx, mp); err != nil {
		s.Fatal("Failed to launch and apply proxy: ", err)
	}
	defer cr.CleanupProxy(cleanupCtx)

	// TODO: Add test logic here to monitor network traffic.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.google.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
}
