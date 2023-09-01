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
	"go.chromium.org/tast-tests/cros/local/chrome/proxy/mitmproxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const mitmdumpBinFile = "mitmdump_bin"

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkMonitor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that network requests go through local proxy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"shengjun@google.com",
			"donnadionne@google.com",
		},
		BugComponent: "b:1129862", // ChromeOS > Privacy > DPChromeOS > DPChromeOS Engineering
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Data:         []string{mitmdumpBinFile},
		Fixture:      fixture.ChromeLoggedIn,
	})
}

func NetworkMonitor(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	mp := mitmproxy.New()
	mp.SetBinaryPath(s.DataPath(mitmdumpBinFile)).SetDumpDir(s.OutDir())

	cleanup, err := cr.LaunchAndApplyProxy(ctx, mp)
	if err != nil {
		s.Fatal("Failed to launch and apply proxy: ", err)
	}
	defer cleanup(cleanupCtx)

	// TODO: Add test logic here to monitor network traffic.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.google.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
}
