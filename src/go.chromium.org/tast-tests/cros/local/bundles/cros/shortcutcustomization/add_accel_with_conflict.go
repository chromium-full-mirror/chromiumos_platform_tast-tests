// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
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
		Func:         AddAccelWithConflict,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Add accelerator for unlocked actions which has conflict ,resolving it and verify its working as expected",
		Contacts: []string{
			"cros-peripherals@google.com",
			"jimmyxgong@google.com",
			"bhattacharyaar@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Shortcuts
		BugComponent: "b:1131848",
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

func AddAccelWithConflict(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	cr, err := chrome.New(ctx, chrome.EnableFeatures("ShortcutCustomization"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
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

	// Verify default shortcut works and does the intended operation.
	if err := kb.Accel(ctx, "search+c"); err != nil {
		s.Fatal("Failed pressing shortcut for widget: ", err)
	}

	// Verify the right widget was triggered.
	widgetNode := nodewith.Name("Calendar").Role(role.StaticText)
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget: ", err)
	}

	// Closing widget with default shortcut.
	if err := kb.Accel(ctx, "search+c"); err != nil {
		s.Fatal("Failed pressing accel to close the widget: ", err)
	}

	// Launch Shortcut Customization app from launcher search.
	if err := launcher.SearchAndLaunchWithQuery(
		tconn, kb, "Shortcuts", apps.ShortcutCustomization.Name)(ctx); err != nil {
		s.Fatal("Failed to search and launch app: ", err)
	}

	// Verify shortcut customization app is launched.
	if err := sc.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Regex for possible default values for the action.
	defaultShortcutRegex := regexp.MustCompile("(search|launcher) c")

	// Capture the default shortcut node based on possible values.
	defaultShortcut := nodewith.NameRegex(defaultShortcutRegex).Role(role.GenericContainer).First()

	// Clicking edit icon next to the shortcut listing.
	editButton := nodewith.ClassName("edit-button").Ancestor(defaultShortcut)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to find edit button: ", err)
	}

	// Verify the edit dialog is open.
	editDialog := nodewith.Name("Open/close calendar").Role(role.Heading)
	if err := ui.WaitUntilExists(editDialog)(ctx); err != nil {
		s.Fatal("Failed to find the Edit dialog: ", err)
	}

	// Locate the existing default shortcut.
	defaultAccel := nodewith.NameRegex(defaultShortcutRegex).Role(role.GenericContainer)
	if err := ui.WaitUntilExists(defaultAccel)(ctx); err != nil {
		s.Fatal("Failed to find default accel for Open/close calendar: ", err)
	}

	// Click Add shortcut button.
	addItemButton := nodewith.Name("Add shortcut").Role(role.Button)
	if err := ui.LeftClick(addItemButton)(ctx); err != nil {
		s.Fatal("Failed to find Add shortcut: ", err)
	}

	// Input the new accel in the shortcut app.
	if err := kb.Accel(ctx, "search+ctrl+s"); err != nil {
		s.Fatal("Failed to input the accel: ", err)
	}

	// Verify error message appears due to the conflict
	conflictMessage := "Shortcut is being used for \"Open Key Shortcuts app\". Press a new shortcut. To replace the original shortcut, press this shortcut again."
	errorMessage := nodewith.Name(conflictMessage).Role(role.StaticText).First()
	if err := ui.WaitUntilExists(errorMessage)(ctx); err != nil {
		s.Fatal("Failed to find the error message indicating conflict: ", err)
	}

	// Input the new accel in the shortcut app again to override.
	if err := kb.Accel(ctx, "search+ctrl+s"); err != nil {
		s.Fatal("Failed to input the accel: ", err)
	}

	// Close the edit dialog.
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to find Done button and close the dialog: ", err)
	}

	// Verify the new accel is now available in the shortcut app.
	if err := sc.VerifyShortcutsRegex(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: "(search|launcher) ctrl s", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl alt m for the shortcut: ", err)
	}

	// Verify the conflicting shortcut is not available for the original action
	// Close the shortcut app.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w: ", err)
	}

	// Verify the shortcut works and does the intended operation.
	if err := kb.Accel(ctx, "search+ctrl+s"); err != nil {
		s.Fatal("Failed pressing alt+shift+s : ", err)
	}

	// Verify the widget opens up.
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget with the new shortcut: ", err)
	}

	// Verify default shortcut also triggers the action.
	if err := kb.Accel(ctx, "search+c"); err != nil {
		s.Fatal("Failed pressing search+c : ", err)
	}

	// Verify that the widget is closed.
	if err := ui.WaitUntilGone(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to close widget with the new shortcut: ", err)
	}
}
