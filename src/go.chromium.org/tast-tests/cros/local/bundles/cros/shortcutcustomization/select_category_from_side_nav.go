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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	sc "go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelectCategoryFromSideNav,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Categories from side nav are selectable",
		Contacts: []string{
			"cros-peripherals@google.com",
			"longbowei@google.com",
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
	})
}

func SelectCategoryFromSideNav(ctx context.Context, s *testing.State) {
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
	shortcutCustomizationRootNode, err := sc.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch shortcut customization app: ", err)
	}

	// Verify shortcut customization app is launched and categories are visible.
	if err := sc.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Shortcuts app opened to the “General” category by default.
	// Verify "General" subcategories are visible.
	generalSubcategories := []string{"General controls", "Apps"}
	if err := sc.VerifySubcategory(ctx, ui, generalSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within General category: ", err)
	}
	// Verify shortcuts within "General" are visible.
	// TODO(longbowei): Include tests to validate accelerators that have the "meta" key.
	// Currently, we display the "meta" key as an icon, and we may have errors because
	// devices will show either "search" icon or "launcher" icon.
	shortcutsInGeneralCategory := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Open notifications", sc.ShortcutKeys{Keys: "alt shift n", Role: role.GenericContainer}},
		{"Open Crosh window", sc.ShortcutKeys{Keys: "ctrl alt t", Role: role.GenericContainer}},
		{"Open feedback tool", sc.ShortcutKeys{Keys: "alt shift i", Role: role.GenericContainer}},
	}
	for _, shortcut := range shortcutsInGeneralCategory {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within General category: ", err)
		}
	}

	// Select "Device" from the side nav.
	deviceCategory := nodewith.Name("Device").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(deviceCategory)(ctx); err != nil {
		s.Fatal("Failed to click Device category: ", err)
	}
	// Verify subcategories within "Device" are visible.
	deviceSubcategories := []string{"Media", "Inputs", "Display"}
	if err := sc.VerifySubcategory(ctx, ui, deviceSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within device category: ", err)
	}
	// Verify shortcuts within "Device" are visible.
	shortcutsInDeviceCategory := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Reset zoom level", sc.ShortcutKeys{Keys: "ctrl shift 0", Role: role.GenericContainer}},
		{"Toggle stylus tools", sc.ShortcutKeys{Keys: "alt shift p", Role: role.GenericContainer}},
		{"Switch to last language selected", sc.ShortcutKeys{Keys: "ctrl space", Role: role.GenericContainer}},
	}
	for _, shortcut := range shortcutsInDeviceCategory {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Device category: ", err)
		}
	}

	// Select "Browser" from the side nav.
	browserCategory := nodewith.Name("Browser").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(browserCategory)(ctx); err != nil {
		s.Fatal("Failed to click Browser category: ", err)
	}
	// Verify subcategories within "Browser" are visible.
	browserSubcategories := []string{"General", "Browser navigation", "Pages", "Tabs", "Bookmarks", "Developer tools"}
	if err := sc.VerifySubcategory(ctx, ui, browserSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within browser category: ", err)
	}
	// Verify shortcuts within "Browser" are visible.
	shortcutsInBrowserCategory := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Open History page", sc.ShortcutKeys{Keys: "ctrl h", Role: role.GenericContainer}},
		{"Open Downloads page", sc.ShortcutKeys{Keys: "ctrl j", Role: role.GenericContainer}},
		{"Open file in Chrome browser", sc.ShortcutKeys{Keys: "ctrl o", Role: role.GenericContainer}},
	}
	for _, shortcut := range shortcutsInBrowserCategory {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Browser category: ", err)
		}
	}

	// Select "Text" from the side nav.
	textCategory := nodewith.Name("Text").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(textCategory)(ctx); err != nil {
		s.Fatal("Failed to click Text category: ", err)
	}
	// Verify subcategories within "Text" are visible.
	textSubcategories := []string{"Text editing", "Text navigation"}
	if err := sc.VerifySubcategory(ctx, ui, textSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within text category: ", err)
	}
	// Verify shortcuts within "Text" are visible.
	shortcutsInTextCategory := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Copy selected content to clipboard", sc.ShortcutKeys{Keys: "ctrl c", Role: role.GenericContainer}},
		{"Select everything on page", sc.ShortcutKeys{Keys: "ctrl a", Role: role.GenericContainer}},
		{"Undo last action", sc.ShortcutKeys{Keys: "ctrl z", Role: role.GenericContainer}},
	}
	for _, shortcut := range shortcutsInTextCategory {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Text category: ", err)
		}
	}

	// Select "Windows and desks" from the side nav.
	windowsAndDesksCategory := nodewith.Name("Windows and desks").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(windowsAndDesksCategory)(ctx); err != nil {
		s.Fatal("Failed to click Windows and desks category: ", err)
	}
	// Verify subcategories within "Windows and desks" are visible.
	windowsAndDesksSubcategories := []string{"Windows", "Desks"}
	if err := sc.VerifySubcategory(ctx, ui, windowsAndDesksSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within Windows and desks category: ", err)
	}
	// Verify shortcuts within "Windows and desks" are visible.
	shortcutsInWindowsAndDesksCategory := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Maximize window", sc.ShortcutKeys{Keys: "alt =", Role: role.GenericContainer}},
		{"Minimize window", sc.ShortcutKeys{Keys: "alt -", Role: role.GenericContainer}},
		{"Close current window", sc.ShortcutKeys{Keys: "ctrl shift w", Role: role.GenericContainer}},
	}
	for _, shortcut := range shortcutsInWindowsAndDesksCategory {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Windows and desks category: ", err)
		}
	}

	// Select "Accessibility" from the side nav.
	accessibilityCategory := nodewith.Name("Accessibility").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(accessibilityCategory)(ctx); err != nil {
		s.Fatal("Failed to click Accessibility category: ", err)
	}
	// Verify subcategories within "Accessibility" are visible.
	accessibilitySubcategories := []string{"ChromeVox", "Visibility", "Accessibility navigation"}
	if err := sc.VerifySubcategory(ctx, ui, accessibilitySubcategories); err != nil {
		s.Fatal("Failed to find subcategories within accessibility category: ", err)
	}
	// Verify shortcuts within "Accessibility" are visible.
	shortcutsInAccessibilityCategory := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Highlight Launcher button on shelf", sc.ShortcutKeys{Keys: "alt shift l", Role: role.GenericContainer}},
		{"Move focus to pop-ups and dialogs", sc.ShortcutKeys{Keys: "alt shift a", Role: role.GenericContainer}},
		{"Select first icon to the left of address bar", sc.ShortcutKeys{Keys: "alt shift t", Role: role.GenericContainer}},
	}
	for _, shortcut := range shortcutsInAccessibilityCategory {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Accessibility category: ", err)
		}
	}
}
