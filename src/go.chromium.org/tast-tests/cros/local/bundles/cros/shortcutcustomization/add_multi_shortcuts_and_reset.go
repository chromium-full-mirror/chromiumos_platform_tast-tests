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
		Func:         AddMultiShortcutsAndReset,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Add Custom Accelerator to multiple actions and reset to default for only one",
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
func AddMultiShortcutsAndReset(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ShortcutCustomization"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

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
	defer kb.Close(cleanupCtx)

	// Launch Shortcut Customization app from launcher search.
	if err := launcher.SearchAndLaunchWithQuery(
		tconn, kb, "Shortcuts", apps.ShortcutCustomization.Name)(ctx); err != nil {
		s.Fatal("Failed to search and launch app: ", err)
	}

	// Verify shortcut customization app is launched.
	if err := sc.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Capture the default shortcut node based on possible values for Open Calendar
	defaultCalendarShortcut := nodewith.NameRegex(regexp.MustCompile("(search|launcher) c")).Role(role.GenericContainer).First()

	// Verify default shortcut is present in the key shortcuts UI.
	if err := sc.VerifyShortcutsRegex(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: "(search|launcher) c", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the Open/close calendar: ", err)
	}

	// Click the edit button for the shortcut.
	editButton := nodewith.ClassName("edit-button").Ancestor(defaultCalendarShortcut)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to find edit button: ", err)
	}

	// Add custom shortcut for open/close calendar.
	if err := sc.AddCustomShortcut(ctx, ui, kb, "Open/close calendar", "ctrl+alt+m", sc.WarnMessageNoSearch); err != nil {
		s.Fatal("Failed to add the ctrl alt m for the shortcut: ", err)
	}

	// Verify the new accel for Calendar is now available in the shortcut app.
	if err := sc.VerifyShortcuts(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: "ctrl alt m", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl alt m for the shortcut: ", err)
	}

	// Verify default shortcut "Take full screenshot or screen recording" present in the key shortcuts UI.
	defaultScreenshotShortcut := nodewith.NameRegex(regexp.MustCompile("ctrl overview|screenshot")).Role(role.GenericContainer).First()
	if err := sc.VerifyShortcutsRegex(ctx, ui, "Take full screenshot or screen recording", sc.ShortcutKeys{Keys: "ctrl overview|screenshot", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the Screenshot shortcut: ", err)
	}

	// Click the edit button for "Take full screenshot or screen recording" shortcut.
	editButton = nodewith.ClassName("edit-button").Ancestor(defaultScreenshotShortcut)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to find edit button: ", err)
	}

	// Add the custom shortcut to the action
	if err := sc.AddCustomShortcut(ctx, ui, kb, "Take full screenshot or screen recording", "ctrl+search+o", ""); err != nil {
		s.Fatal("Failed to add the ctrl search o for the shortcut: ", err)
	}
	// Verify the new accel for Screenshot is now available in the shortcut app.
	if err := sc.VerifyShortcutsRegex(ctx, ui, "Take full screenshot or screen recording", sc.ShortcutKeys{Keys: "(search|launcher) ctrl o", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl search o for the shortcut: ", err)
	}

	// Close Key shortcut app.
	if err := kb.Accel(ctx, "ctrl+w"); err != nil {
		s.Fatal("Failed press ctrl w and close the window : ", err)
	}

	// Verify the intended widgets are launched with the new shortcuts.
	if err := uiauto.Combine("Lauching the widgets with the custom shortcut",
		kb.AccelAction("ctrl+alt+m"),
		ui.WaitUntilExists(nodewith.Name("Calendar").Role(role.StaticText)),
		kb.AccelAction("ctrl+search+o"),
		ui.WaitUntilExists(nodewith.Name("Screenshot taken").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to find and click the shortcut app in launcher: ", err)
	}

	// Relaunch Key shortcut app.
	if err := launcher.SearchAndLaunchWithQuery(
		tconn, kb, "Shortcuts", apps.ShortcutCustomization.Name)(ctx); err != nil {
		s.Fatal("Failed to search and launch app: ", err)
	}

	// Verify shortcut customization app is launched.
	if err := sc.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Verify the new accel is now available in the shortcut app.
	if err := sc.VerifyShortcuts(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: "ctrl alt m", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl alt m for the shortcut: ", err)
	}

	// Click Edit button next to the "Open/close calendar" again, to reset the shortcut.
	editButton = nodewith.ClassName("edit-button").Ancestor(defaultCalendarShortcut)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to find edit button: ", err)
	}

	// Click restore defaults for "Open/Close calendar" customized shortcut and close the dialog.
	if err := uiauto.Combine("Open Edit dialog and click reset for the specific widget",
		ui.WaitUntilExists(nodewith.Name("Restore defaults").Role(role.StaticText)),
		ui.LeftClick(nodewith.Name("Restore defaults").Role(role.StaticText)),
		ui.LeftClick(nodewith.Name("Done").Role(role.Button)),
	)(ctx); err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to find edit button: ", err)
	}

	// Checking resetting to default works as intended for the calendar widget.
	// Also making sure other action still works with custom shortcut.
	if err := uiauto.Combine("Checking calendar uses default and screenshot uses custom",
		kb.AccelAction("search+c"),
		ui.WaitUntilExists(nodewith.Name("Calendar").Role(role.StaticText)),
		kb.AccelAction("ctrl+search+o"),
		ui.WaitUntilExists(nodewith.Name("Screenshot taken").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to reset the individual shortcut: ", err)
	}

	// Checking the previsouly customized shortcut is not working for calendar
	// by verifying the calendar widget is not closed on pressing it.
	if err := uiauto.Combine("Check custom shortcut not working any more",
		kb.AccelAction("ctrl+alt+m"),
		ui.WaitUntilExists(nodewith.Name("Calendar").Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to reset the calendar shortcut: ", err)
	}

}
