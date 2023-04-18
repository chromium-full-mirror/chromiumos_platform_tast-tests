// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/launcher"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchFromLauncher,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Shortcut Customization app can be found and launched from a search in launcher",
		Contacts: []string{
			"cros-peripherals@google.com",
			"longbowei@google.com",
			"jimmyxgong@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Shortcuts
		BugComponent: "b:1131848",
		Fixture:      "chromeLoggedInWithShortcutCustomizationApp",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
	})
}

// LaunchFromLauncher verifies launching Shortcut Customization app from the launcher.
func LaunchFromLauncher(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Launch Shortcut Customization app from launcher search.
	query := "go to next tab"
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	// Search for shortcut using launcher and attempt to click result.
	shortcutsSearchItem := launcher.SearchResultListItemFinder.NameContaining("Shortcuts")

	if err := uiauto.Combine(fmt.Sprintf("search %q in launcher", query),
		launcher.Open(tconn),
		launcher.Search(tconn, kb, query),
		ui.WaitUntilExists(shortcutsSearchItem),
		ui.DoDefault(shortcutsSearchItem),
	)(ctx); err != nil {
		s.Fatalf("Failed to search query (%q) and click result (%p)", query, shortcutsSearchItem)
	}

	// TODO(b/278574672): Add more tests that verify opening the app and refactor
	// code into a shared function located in a common place.
	// Verify shortcut app appears in shelf.
	if err := ash.WaitForApp(ctx, tconn, apps.ShortcutCustomization.ID, time.Minute); err != nil {
		s.Fatal("Shortcut Customization app did not appear in shelf after launch: ", err)
	}

	// Verify categories exist.
	for _, name := range []string{"General", "Device", "Browser", "Text", "Windows and Desks", "Accessibility"} {
		if err := uiauto.Combine(fmt.Sprintf("Verify %q category exists", name),
			ui.WaitUntilExists(nodewith.Name(name).Role(role.Button)),
		)(ctx); err != nil {
			s.Fatalf("Failed to find %q category: ", name)
		}
	}

	// Veirfy essential elements exist.
	keyboardsettinglink := nodewith.Name("Keyboard settings").Role(role.Link)
	searchBar := nodewith.Name("Search shortcuts").Role(role.SearchBox)
	if err := uiauto.Combine("Verify essential elements exist",
		ui.WaitUntilExists(keyboardsettinglink),
		ui.WaitUntilExists(searchBar),
	)(ctx); err != nil {
		s.Fatal("Failed to find element: ", err)
	}
}
