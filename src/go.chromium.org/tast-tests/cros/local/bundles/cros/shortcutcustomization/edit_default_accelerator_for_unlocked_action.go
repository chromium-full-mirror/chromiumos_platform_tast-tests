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
		Func:         EditDefaultAcceleratorForUnlockedAction,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Edit default Accelerator for Unlocked actions and verifying associated action is triggered",
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

func EditDefaultAcceleratorForUnlockedAction(ctx context.Context, s *testing.State) {
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
	if err := kb.Accel(ctx, "Search+c"); err != nil {
		s.Fatal("Failed pressing shortcut for widget: ", err)
	}

	// Verify the widget opened from the above step.
	widgetNode := nodewith.Name("Calendar").Role(role.StaticText)
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget: ", err)
	}

	// Verify the new shortcut does not trigger the operation
	// before adding it and only the default works.
	if err := uiauto.Combine("Input accel and check only default shortcut works and not the custom one",
		kb.AccelAction("ctrl+alt+m"),
		ui.WaitUntilExists(widgetNode),
		kb.AccelAction("Search+c"),
	)(ctx); err != nil {
		s.Fatal("Failed to verify only the default shortcut works and custom does not work: ", err)
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
	ShortcutNodeInfo, err := ui.Info(ctx, defaultShortcut)
	if err != nil {
		s.Fatal("Failed to find the default shortcut keys")
	}
	shortcutName := ShortcutNodeInfo.Name

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

	//Verify editing the default shortcut and input the new accel.
	if err := uiauto.Combine("Open the Edit dialog and edit default shortcut",
		ui.WaitUntilExists(nodewith.Name(shortcutName).Role(role.GenericContainer)),
		ui.LeftClick(nodewith.ClassName("clickable-button").Ancestor(editDialog).First()),
		kb.AccelAction("ctrl+alt+m"),
		ui.LeftClick(nodewith.Name("Done").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to edit the default shortcut: ", err)
	}

	// Verify the new accel is now available in the shortcut app.
	if err := sc.VerifyShortcuts(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: "ctrl alt m", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl alt m for the shortcut: ", err)
	}

	// Close the shortcut app.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w: ", err)
	}

	// Verify the custom shortcut works and does the intended operation
	if err := uiauto.Combine("Open the widget with the custom shortcut ",
		kb.AccelAction("ctrl+alt+m"),
		ui.WaitUntilExists(widgetNode),
	)(ctx); err != nil {
		s.Fatal("Failed to verify only the default shortcut works: ", err)
	}

	// Verify default shortcut does not trigger any action.
	if err := uiauto.Combine("Open the widget with the custom shortcut ",
		kb.AccelAction("search+c"),
		ui.WaitUntilExists(widgetNode),
	)(ctx); err != nil {
		s.Fatal("Failed to verify default shortcut does not work any more: ", err)
	}

}
