// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenKeyboardSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open keyboard settings from the Shortcuts app",
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
	})
}

// OpenKeyboardSettings verifies clicking keyboard settings link from shortcut
// customization app will navigate to keyboard subpage of device setting page.
func OpenKeyboardSettings(ctx context.Context, s *testing.State) {
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

	// Launch shortcut customization app.
	shortcutCustomizationRootNode, err := shortcutcustomization.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch shortcut customization app: ", err)
	}

	// Verify shortcut customization app is launched and categories, searchbar
	// and keyboard settings link are visible.
	if err := shortcutcustomization.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Click keyboard settings link.
	keyboardsettingslink := nodewith.Name("Keyboard settings").Role(role.Link).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(keyboardsettingslink)(ctx); err != nil {
		s.Fatal("Failed to click keyboard settings link: ", err)
	}
	// Verify that the Settings app is launched.
	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		s.Fatal("Settings app did not appear in shelf: ", err)
	}
	// Verify that essential elements exist.
	viewKeyboardShortcutsLink := nodewith.Name("View keyboard shortcuts").Role(role.Link)
	changeInputSettingsLink := nodewith.Name("Change input settings").Role(role.Link)
	if err := uiauto.Combine("Verify essential elements exist",
		ui.WaitUntilExists(viewKeyboardShortcutsLink),
		ui.WaitUntilExists(changeInputSettingsLink),
	)(ctx); err != nil {
		s.Fatal("Failed to find element: ", err)
	}
}
