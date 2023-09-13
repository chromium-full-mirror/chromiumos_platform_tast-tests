// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchForShortcuts,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Searching for shortcuts (todo)",
		Contacts: []string{
			"cros-peripherals@google.com",
			"longbowei@google.com",
			"cambickel@google.com",
			"jimmyxgong@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Shortcuts
		BugComponent: "b:1131848",
		Fixture:      "chromeLoggedInWithShortcutCustomizationApp",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-e65827ce-fa73-4956-94b8-cac706f0eb15",
			},
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Timeout:      4 * time.Minute,
	})
}

func SearchForShortcuts(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Launch shortcut customization app.
	shortcutCustomizationRootNode, err := shortcutcustomization.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch shortcut customization app: ", err)
	}

	// Verify shortcut customization app is launched and categories are visible.
	if err := shortcutcustomization.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Focus the search box.
	searchBox := nodewith.Name("Search shortcuts").Role(role.SearchBox).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(searchBox)(ctx); err != nil {
		s.Fatal("Failed to click search box: ", err)
	}

	if err := ui.WaitUntilExists(searchBox.Focused())(ctx); err != nil {
		s.Fatal("Failed to wait for cursor be focused on the search field in Shortcuts app: ", err)
	}

	// Search for a query that should have no results, and indicate that we
	// expect the "No search results" text to appear.
	noResultsQuery := shortcutcustomization.ShortcutsSearchQueryAndExpectation{
		Query:                    "has no results",
		ExpectedDescriptionRegex: "",
		ExpectNoResults:          true,
	}
	_, err = shortcutcustomization.SearchAndCheck(ctx, ui, kb, noResultsQuery)
	if err != nil {
		s.Fatal("Failed to search with query: ", err)
	}
	if err := shortcutcustomization.ClearSearch(ctx, ui)(ctx); err != nil {
		s.Fatal("Failed to clear search: ", err)
	}

	// Search for a shortcut that has a result on a different page,
	// then click on it to verify that navigation works.
	closeWindowQuery := shortcutcustomization.ShortcutsSearchQueryAndExpectation{
		Query:                    "close current window",
		ExpectedDescriptionRegex: "Close current window",
		ExpectNoResults:          false,
	}
	result, err := shortcutcustomization.SearchAndCheck(ctx, ui, kb, closeWindowQuery)
	if err != nil {
		s.Fatal("Failed to search with query: ", err)
	}

	// Select that result, which should navigate to the "Windows and desks" page.
	if err := mouse.Click(tconn, result.Location.CenterPoint(), mouse.LeftButton)(ctx); err != nil {
		s.Fatal("Failed to click on search result: ", err)
	}
	// Verify subcategories within "Windows and desks" are visible, since we should
	// now be on that page after clicking the search result.
	windowsAndDesksSubcategories := []string{"Windows", "Desks"}
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, windowsAndDesksSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within Windows and desks category: ", err)
	}
}
