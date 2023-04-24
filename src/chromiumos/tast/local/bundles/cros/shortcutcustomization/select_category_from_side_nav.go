// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/shortcutcustomization"
	"chromiumos/tast/local/input"
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

	// Shortcuts app opened to the “General” category by default.
	// Verify "General" subcategories are visible.
	generalSubcategories := []string{"General Controls", "Apps"}
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, generalSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within General category: ", err)
	}
	// Verify shortcuts within "General" are visible.
	// TODO(longbowei): Include tests to validate accelerators that have the "meta" key.
	// Currently, we display the "meta" key as an icon, and we may have errors because
	// devices will show either "search" icon or "launcher" icon.
	shortcutsInGeneralCategory := []struct {
		description string
		keys        string
	}{
		{"Open notifications", "alt shift n"},
		{"Open Crosh window", "ctrl alt t"},
		{"Submit feedback", "alt shift i"},
	}
	for _, shortcut := range shortcutsInGeneralCategory {
		if err := shortcutcustomization.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
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
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, deviceSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within device category: ", err)
	}
	// Verify shortcuts within "Device" are visible.
	shortcutsInDeviceCategory := []struct {
		description string
		keys        string
	}{
		{"Switch to next available input method", "ctrl shift space"},
		{"Show stylus tools", "alt shift p"},
		{"Switch to last language selected", "ctrl space"},
	}
	for _, shortcut := range shortcutsInDeviceCategory {
		if err := shortcutcustomization.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
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
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, browserSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within browser category: ", err)
	}
	// Verify shortcuts within "Browser" are visible.
	shortcutsInBrowserCategory := []struct {
		description string
		keys        string
	}{
		{"Open History page", "ctrl h"},
		{"Open Downloads page", "ctrl j"},
		{"Open file in Chrome browser", "ctrl o"},
	}
	for _, shortcut := range shortcutsInBrowserCategory {
		if err := shortcutcustomization.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
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
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, textSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within text category: ", err)
	}
	// Verify shortcuts within "Text" are visible.
	shortcutsInTextCategory := []struct {
		description string
		keys        string
	}{
		{"Copy selected content to clipboard", "ctrl c"},
		{"Select everything on page", "ctrl a"},
		{"Undo last action", "ctrl z"},
	}
	for _, shortcut := range shortcutsInTextCategory {
		if err := shortcutcustomization.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Text category: ", err)
		}
	}

	// Select "Windows and Desks" from the side nav.
	windowsAndDesksCategory := nodewith.Name("Windows and Desks").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(windowsAndDesksCategory)(ctx); err != nil {
		s.Fatal("Failed to click Windows and Desks category: ", err)
	}
	// Verify subcategories within "Windows and Desks" are visible.
	windowsAndDesksSubcategories := []string{"Windows", "Desks"}
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, windowsAndDesksSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within windows and desks category: ", err)
	}
	// Verify shortcuts within "Windows and Desks" are visible.
	shortcutsInWindowsAndDesksCategory := []struct {
		description string
		keys        string
	}{
		{"Maximize window", "alt ="},
		{"Minimize window", "alt -"},
		{"Close current window", "ctrl shift w"},
	}
	for _, shortcut := range shortcutsInWindowsAndDesksCategory {
		if err := shortcutcustomization.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Windows and Desks category: ", err)
		}
	}

	// Select "Accessibility" from the side nav.
	accessibilityCategory := nodewith.Name("Accessibility").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(accessibilityCategory)(ctx); err != nil {
		s.Fatal("Failed to click Accessibility category: ", err)
	}
	// Verify subcategories within "Accessibility" are visible.
	accessibilitySubcategories := []string{"ChromeVox", "Visibility", "Accessibility navigation"}
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, accessibilitySubcategories); err != nil {
		s.Fatal("Failed to find subcategories within accessibility category: ", err)
	}
	// Verify shortcuts within "Accessibility" are visible.
	shortcutsInAccessibilityCategory := []struct {
		description string
		keys        string
	}{
		{"Highlight launcher button on shelf", "alt shift l"},
		{"Move focus to popups and dialogs", "alt shift a"},
		{"Select first icon to the left of address bar", "alt shift t"},
	}
	for _, shortcut := range shortcutsInAccessibilityCategory {
		if err := shortcutcustomization.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to find shortcuts within Accessibility category: ", err)
		}
	}
}
