// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeleteBrowserHistory,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that a Unicorn Account can delete browsing history",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"agawronska@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Vars:         []string{"unicorn.allowedSite", "unicorn.allowedSiteTitle"},
		Params: []testing.Param{{
			Fixture: "familyLinkUnicornLogin",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "familyLinkUnicornLoginWithLacros",
			Val:               browser.TypeLacros,
		}},
	})
}

const siteURL = "https://www.google.com/"
const siteTitle = "Google"

// DeleteBrowserHistory validates that browser history deletion works for Unicorn accounts.
func DeleteBrowserHistory(ctx context.Context, s *testing.State) {
	// Reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), siteURL)
	if err != nil {
		s.Fatal("Failed to navigate to site: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	testing.ContextLog(ctx, "Checking for site in browsing history")
	if err = conn.Navigate(ctx, "chrome://history"); err != nil {
		s.Fatal("Failed to navigate to chrome://history: ", err)
	}
	ui := uiauto.New(tconn)
	link := nodewith.Name(siteTitle).Role(role.Link)
	if err = ui.WaitUntilExists(link)(ctx); err != nil {
		s.Fatal("Site not found in history: ", err)
	}

	testing.ContextLog(ctx, "Attempting to delete browsing history")
	if err = conn.Navigate(ctx, "chrome://settings/clearBrowserData"); err != nil {
		s.Fatal("Failed to navigate to chrome://settings/clearBrowserData: ", err)
	}
	if err = ui.DoDefault(nodewith.ClassName("action-button").Name("Clear data"))(ctx); err != nil {
		s.Fatal("Failed to click 'Clear data' button: ", err)
	}

	testing.ContextLog(ctx, "Verifying site no longer present in browsing history")
	if err = conn.Navigate(ctx, "chrome://history"); err != nil {
		s.Fatal("Failed to navigate to chrome://history: ", err)
	}
	// In the case that history deletion fails, there may be a brief lag (usually <5s)
	// before the link appears on the reloaded history page. To avoid reporting a false
	// success, the test checks that the site is persistently gone.
	if err = ui.WaitUntilGoneFor(link, 5*time.Second)(ctx); err != nil {
		s.Fatal("Site remained in history after deletion attempt: ", err)
	}
}
