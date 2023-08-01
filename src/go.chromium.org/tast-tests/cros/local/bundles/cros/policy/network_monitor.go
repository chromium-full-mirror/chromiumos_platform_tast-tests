// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/mitmproxy"
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
		Timeout:      15 * time.Minute,
		Data:         []string{mitmdumpBinFile},
		Fixture:      fixture.ChromeLoggedIn,
	})
}

func NetworkMonitor(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	mitmdumpBin := s.DataPath(mitmdumpBinFile)
	if err := os.Chmod(mitmdumpBin, 0755); err != nil {
		s.Fatalf("Failed to chmod %v: %v", mitmdumpBin, err)
	}

	mp := mitmproxy.New()
	cleanupFunc, certPath, err := mp.SetBinaryPath(s.DataPath(mitmdumpBinFile)).
		SetDumpDir(s.OutDir()).Start(ctx)
	if err != nil {
		s.Fatal("Failed to launch mitmproxy: ", err)
	}
	defer func(ctx context.Context) {
		if err := cleanupFunc(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup mitmproxy: ", err)
		}
	}(cleanupCtx)

	if err := mitmproxy.ImportRootCertificate(ctx, cr.NormalizedUser(), certPath); err != nil {
		s.Fatal("Failed to import cert: ", err)
	}

	if err := cr.SetProxy(ctx, fmt.Sprintf("localhost:%d", mp.ListenPort())); err != nil {
		s.Fatal("Failed to set Chrome proxy: ", err)
	}

	// TODO: Add test logic here to monitor network traffic.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.google.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
}
