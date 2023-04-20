// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/shortcutcustomization"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
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

	// TODO(b/278737545): Currently, the code only verifies the subcategories,
	// also need to verify the descriptions and shortcut keys of specific
	// accelerators within each subcategory.
	// Shortcuts app opened to the “General” category by default.
	// Verify "General" subcategories are visible.
	generalSubcategories := []string{"General Controls", "Apps"}
	if err := shortcutcustomization.VerifySubcategory(ctx, ui, generalSubcategories); err != nil {
		s.Fatal("Failed to find subcategories within General category: ", err)
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
}
