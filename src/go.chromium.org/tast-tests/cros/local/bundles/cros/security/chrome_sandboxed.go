// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChromeSandboxed,
		Desc: "Verify Chrome's sandbox status",
		Contacts: []string{
			"chromeos-hardening@google.com",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		Fixture:      "chromeLoggedIn",
	})
}

func ChromeSandboxed(ctx context.Context, s *testing.State) {
	const (
		url      = "chrome://sandbox/"
		text     = "You are adequately sandboxed."
		waitExpr = "document.getElementsByTagName('p')[0].textContent"
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, url)
	if err != nil {
		s.Fatal("Failed to create a new connection: ", err)
	}
	defer closeBrowser(ctx)
	defer conn.Close()

	{
		ectx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err = conn.WaitForExpr(ectx, waitExpr); err != nil {
			s.Fatalf("Failed to evaluate in %q in %s", waitExpr, url)
		}
	}

	c, err := conn.PageContent(ctx)
	if err != nil {
		s.Fatal("Failed to obtain the page content")
	}

	if !strings.Contains(c, text) {
		s.Errorf("Could not find %q in %q in %s", text, c, url)
	}
}
