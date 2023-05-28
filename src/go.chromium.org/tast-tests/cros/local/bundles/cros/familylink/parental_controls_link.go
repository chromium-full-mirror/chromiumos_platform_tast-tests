// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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
		Func: ParentalControlsLink,
		// TODO(b/250500759): Support verifies 'Parental controls' setting opens the Lacros browser in Lacros mode once this issue is fixed.
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verify 'Parental controls' setting opens https://families.google.com/families when Play Store is disabled",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"agawronska@chromium.org",
			"awendy@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
		Fixture:      "familyLinkGellerLogin", // Expecting ARC to be disabled in this test.
	})
}

const familiesURL = "https://families.google.com/families"

// ParentalControlsLink verifies 'Parental controls' opens https://families.google.com/families.
func ParentalControlsLink(ctx context.Context, s *testing.State) {
	var (
		cr    = s.FixtValue().(chrome.HasChrome).Chrome()
		tconn = s.FixtValue().(familylink.HasTestConn).TestConn()
		ui    = uiauto.New(tconn)
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	if err := uiauto.Combine("open parental controls",
		ui.LeftClick(nodewith.NameContaining("Parental controls Open").FinalAncestor(ossettings.WindowFinder)),
		ui.WaitUntilExists(nodewith.NameContaining("Families").HasClass("BrowserFrame")),
	)(ctx); err != nil {
		s.Fatal(`Failed to verify the functionality of "Parental controls" settings: `, err)
	}

	tabs, err := browser.CurrentTabs(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find current tabs: ", err)
	}

	if len(tabs) != 1 {
		s.Fatalf("Failed to verify the expected page opened: unexpected tab number: want 1, got %d", len(tabs))
	}

	if tabs[0].URL != familiesURL {
		s.Fatalf("Failed to verify the expected page opened: want %q, got %q", familiesURL, tabs[0].URL)
	}
}
