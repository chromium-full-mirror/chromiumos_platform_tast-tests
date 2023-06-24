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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AddCustomAcceleratorAndReset,
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
	})

}
func AddCustomAcceleratorAndReset(ctx context.Context, s *testing.State) {
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
	if err := kb.Accel(ctx, "alt+shift+s"); err != nil {
		s.Fatal("Failed pressing alt+shift+s : ", err)
	}

	widgetNode := nodewith.NameRegex(regexp.MustCompile("(?i)sign out")).Role(role.Button)
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget : ", err)
	}

	// Verify the new shortcut to be input, does not trigger the action
	// before adding it.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed pressing ctrl+alt+m : ", err)
	}

	// Verify the widget is still open as the new shortcut will not close it.
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget as must have go closed : ", err)
	}

	// Closing widget with default shortcut.
	if err := kb.Accel(ctx, "alt+shift+s"); err != nil {
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

	// Verify default shortcut is present in the key shortcuts UI.
	if err := sc.VerifyShortcuts(ctx, ui, "Open Quick Settings by selecting the time", sc.ShortcutKeys{Keys: "alt shift s", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the Open Quick Settings shortcut: ", err)
	}

	// Click the edit button for the shortcut.
	openWidget := nodewith.Name("alt shift s").Role(role.GenericContainer)
	editButton := nodewith.ClassName("edit-button").Ancestor(openWidget)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to find edit button: ", err)
	}

	// Verify the edit dialog is opened correctly.
	editDialog := nodewith.NameContaining("Open Quick Settings").Role(role.Dialog)
	if err := ui.WaitUntilExists(editDialog)(ctx); err != nil {
		s.Fatal("Failed to find the Edit dialog: ", err)
	}

	// Verify the default shortcut is present.
	defaultAccel := nodewith.Name("alt shift s").Role(role.GenericContainer)
	if err := ui.WaitUntilExists(defaultAccel)(ctx); err != nil {
		s.Fatal("Failed to find default accel for the action: ", err)
	}

	// Click Add shortcut button.
	addItemButton := nodewith.Name("Add shortcut").Role(role.Button)
	if err := ui.LeftClick(addItemButton)(ctx); err != nil {
		s.Fatal("Failed to find Add item: ", err)
	}

	// Input the new shortcut.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed to input the accel: ", err)
	}

	// Close the edit dialog by clicking the done button.
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := ui.LeftClick(doneButton)(ctx); err != nil {
		s.Fatal("Failed to find Done button and close the dialog : ", err)
	}

	// Verify the new accel is now available in the shortcut app.
	if err := sc.VerifyShortcuts(ctx, ui, "Open Quick Settings by selecting the time", sc.ShortcutKeys{Keys: "ctrl alt m", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the ctrl alt m for the shortcut: ", err)
	}

	// Verify the new shortcut works as intended.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed pressing ctrl+alt+m : ", err)
	}

	// Verify the widget opens after pressing the shortcut combo.
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the customized widget : ", err)
	}

	// Press shortcut again to close the widget.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed to press ctrl+alt+m: ", err)
	}

	// Click the reset all button to restore the defaults.
	allresetButton := nodewith.Name("Reset all shortcuts").Role(role.Button)
	if err := ui.LeftClick(allresetButton)(ctx); err != nil {
		s.Fatal("Failed to find reset button: ", err)
	}

	// Click the reset confirmation button to switch to have just the default shortcut.
	resetButton := nodewith.Name("Reset").Role(role.Button)
	if err := ui.LeftClick(resetButton)(ctx); err != nil {
		s.Fatal("Failed to find reset button: ", err)
	}

	// Verify original shortcut available after reset.
	if err := sc.VerifyShortcuts(ctx, ui, "Open Quick Settings by selecting the time", sc.ShortcutKeys{Keys: "alt shift s", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find the default shortcut alt shift s: ", err)
	}

	// Verify the edited accelator is not available anymore in UI.
	if err := sc.VerifyShortcuts(ctx, ui, "Open Quick Settings by selecting the time", sc.ShortcutKeys{Keys: "ctrl alt m", Role: role.GenericContainer}); err == nil {
		s.Fatal("Failed to reset the shortcut: ", err)
	}

	// Verify edited accelator combo does not work any more as reset.
	if err := kb.Accel(ctx, "ctrl+alt+m"); err != nil {
		s.Fatal("Failed pressing ctrl+alt+m : ", err)
	}

	// Verify that no widget opens on pressing the edited combo in the previous step.
	if err := ui.WaitUntilGone(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to close the widget with the default shortcut : ", err)
	}
}
