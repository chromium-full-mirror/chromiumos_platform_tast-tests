// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ParentalControlsLink,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify 'Parental controls' setting opens family website when Play Store is disabled",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"amberhaynes@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
		Params: []testing.Param{{
			Fixture: "familyLinkGellerLogin", // Expecting ARC to be disabled in this test.
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "familyLinkGellerLoginWithLacros", // Expecting ARC to be disabled in this test.
			Val:               browser.TypeLacros,
		}},
	})
}

const familyURL = "myaccount.google.com/family/details"

func ParentalControlsLink(ctx context.Context, s *testing.State) {
	var (
		cr    = s.FixtValue().(chrome.HasChrome).Chrome()
		tconn = s.FixtValue().(familylink.HasTestConn).TestConn()
		ui    = uiauto.New(tconn)
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	settings, err := ossettings.LaunchAtPage(ctx, tconn, nodewith.Name("Accounts").Role(role.Link))
	if err != nil {
		s.Fatal("Failed to open Accounts page: ", err)
	}
	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")
		if err := settings.Close(ctx); err != nil {
			s.Log("Failed to close settings: ", err)
		}
	}(cleanupCtx)

	nodeName := "Your family on Google"
	browserWindow := nodewith.NameContaining(nodeName).HasClass("BrowserFrame")
	if s.Param().(browser.Type) == browser.TypeLacros {
		browserWindow = nodewith.NameContaining(nodeName).ClassNameRegex(regexp.MustCompile(`^ExoShellSurface(-\d+)?$`))
	}

	if err := uiauto.Combine("open parental controls",
		ui.LeftClick(nodewith.NameContaining("Parental controls Open").FinalAncestor(ossettings.WindowFinder)),
		ui.WaitUntilExists(browserWindow),
	)(ctx); err != nil {
		s.Fatal(`Failed to verify the functionality of "Parental controls" settings: `, err)
	}

	tabs, err := br.CurrentTabs(ctx)
	if err != nil {
		s.Fatal("Failed to find current tabs: ", err)
	}

	if len(tabs) != 1 {
		s.Fatalf("Failed to verify the expected page opened: unexpected tab number: want 1, got %d", len(tabs))
	}
	if !strings.Contains(tabs[0].URL, familyURL) {
		s.Errorf("Failed to verify the expected page opened: want %q, got %q", familyURL, tabs[0].URL)
	}
}
