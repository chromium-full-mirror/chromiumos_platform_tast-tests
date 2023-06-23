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
		Func:         AddCustomAcceleratorToUnlockedAction,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Add Custom Accelerator to Unlocked actions",
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

func AddCustomAcceleratorToUnlockedAction(ctx context.Context, s *testing.State) {
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
	defer kb.Close(cleanupCtx)

	// Verify default shortcut works and does the intended operation.
	if err := kb.Accel(ctx, "search+c"); err != nil {
		s.Fatal("Failed pressing alt+shift+s : ", err)
	}

	// Verify the right widget was triggered.
	widgetNode := nodewith.Name("Calendar").Role(role.StaticText)
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget: ", err)
	}

	// Verify the new shortcut does not trigger the open notification
	// before adding it.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed pressing ctrl+alt+m : ", err)
	}

	// Verify the widget is still open as the new shortcut will not close it.
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget : ", err)
	}

	// Closing widget with default shortcut.
	if err := kb.Accel(ctx, "search+c"); err != nil {
		s.Fatal("Failed pressing alt+shift+s : ", err)
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
	defaultShortcutRegex := regexp.MustCompile("meta (search|launcher) c")

	// Capture the default shortcut node based on possible values.
	defaultShortcut := nodewith.NameRegex(defaultShortcutRegex).Role(role.GenericContainer).First()

	// Capture the name of the node for shortcut.
	shortcutNodeInfo, err := ui.Info(ctx, defaultShortcut)
	if err != nil {
		s.Fatal("Failed to find the default shortcut keys")
	}
	shortcutName := shortcutNodeInfo.Name

	// Verify shortcut is present in the shortcut app ui.
	if err := sc.VerifyShortcuts(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: shortcutName, Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the shortcut: ", err)
	}

	// Verify to click edit button to input the new shortcut.
	editButton := nodewith.ClassName("edit-button").Ancestor(defaultShortcut)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to find edit button: ", err)
	}

	// Verify the edit dialog is open.
	editDialog := nodewith.Name("Open/close calendar").Role(role.Dialog)
	if err := ui.WaitUntilExists(editDialog)(ctx); err != nil {
		s.Fatal("Failed to find the Edit dialog: ", err)
	}

	//Verify the default accel in the dialog window
	defaultAccel := nodewith.Name(shortcutName).Role(role.GenericContainer)
	if err := ui.WaitUntilExists(defaultAccel)(ctx); err != nil {
		s.Fatal("Failed to find default accel for Open notifications: ", err)
	}

	// Click Add shortcut button.
	addItemButton := nodewith.Name("Add shortcut").Role(role.Button)
	if err := ui.LeftClick(addItemButton)(ctx); err != nil {
		s.Fatal("Failed to find Add item: ", err)
	}

	// Input the new accel in the shortcut app.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed to input the accel: ", err)
	}

	// Close the edit dialog.
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to find Done button and close the dialog : ", err)
	}

	// Verify the new accel is now available in the shortcut app.
	if err := sc.VerifyShortcuts(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: "ctrl alt m", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl alt m for the shortcut: ", err)
	}

	// Close the shortcut app.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w : ", err)
	}

	// Verify the shortcut works and does the intended operation.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed pressing ctrl+alt+m : ", err)
	}

	// Verify the widget opens up.
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget with the new shortcut: ", err)
	}

	// Verify even the default shortcut works.
	if err := kb.Accel(ctx, "search+c"); err != nil {
		s.Fatal("Failed pressing search+c : ", err)
	}

	// Verify the widget closes with the default shortcut
	// to ensure default shortcut still works.
	if err := ui.WaitUntilGone(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to close the widget with the default shortcut : ", err)
	}
}
