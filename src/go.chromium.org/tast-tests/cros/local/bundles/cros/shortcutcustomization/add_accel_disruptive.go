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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
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
		Func:         AddAccelDisruptive,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Add accelerator for unlocked actions which is disruptive and verify it is not added as shortcut ",
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

func AddAccelDisruptive(ctx context.Context, s *testing.State) {
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

	// Verify the widget is triggered.
	widgetNode := nodewith.Name("Calendar").Role(role.StaticText)
	if err := ui.WaitUntilExists(widgetNode)(ctx); err != nil {
		s.Fatal("Failed to find the widget: ", err)
	}

	// Closing widget with default shortcut.
	if err := kb.Accel(ctx, "Search+c"); err != nil {
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
	defaultShortcutRegex := regexp.MustCompile("meta (search|launcher) c")

	// Capture the default shortcut node based on possible values.
	defaultShortcut := nodewith.NameRegex(defaultShortcutRegex).Role(role.GenericContainer).First()

	// Capture the name of the node for shortcut.
	shortcutNodeInfo, err := ui.Info(ctx, defaultShortcut)
	if err != nil {
		s.Fatal("Failed to find the default shortcut keys")
	}
	shortcutName := shortcutNodeInfo.Name

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

	// Locate the existing default shortcut in the edit dialog.
	defaultAccel := nodewith.NameRegex(defaultShortcutRegex).Role(role.GenericContainer)
	if err := ui.WaitUntilExists(defaultAccel)(ctx); err != nil {
		s.Fatal("Failed to find default accel for Open/close calendar: ", err)
	}

	// Click Edit shortcut icon.
	editIcon := nodewith.ClassName("clickable-button").Ancestor(editDialog).First()
	if err := ui.LeftClick(editIcon)(ctx); err != nil {
		s.Fatal("Failed to find Edit icon: ", err)
	}

	// Input the new accel in the shortcut app.
	if err := kb.Accel(ctx, "search+l"); err != nil {
		s.Fatal("Failed to input the accel: ", err)
	}

	// Verify error message appears due to the disruptive shortcut.
	conflictMessage := "Shortcut is being used for \"Lock device\". Press a new shortcut."
	errorMessage := nodewith.Name(conflictMessage).Role(role.StaticText)
	if err := ui.WaitUntilExists(errorMessage)(ctx); err != nil {
		s.Fatal("Failed to find the error message indicating conflict: ", err)
	}

	// Input the new accel in the shortcut app again to try override.
	if err := kb.Accel(ctx, "search+l"); err != nil {
		s.Fatal("Failed to input the accel: ", err)
	}

	// Verify the error message stays after retyping disruptive shortcut.
	errorMessageAgain := nodewith.Name(conflictMessage).Role(role.StaticText)
	if err := ui.WaitUntilExists(errorMessageAgain)(ctx); err != nil {
		s.Fatal("Failed to find the error message indicating conflict: ", err)
	}

	// Verify the lock screen has not appeared.
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err == nil {
		s.Fatalf("Screen locked hence failed: %v (last status %+v)", err, st)
	}

	// Close the edit dialog.
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := ui.WaitUntilExists(doneButton)(ctx); err != nil {
		s.Fatal("Done button not found: ", err)
	}

	// Verify the focus is on the done button and gets clicked.
	if err := uiauto.Combine("find, focus and click the done button", ui.FocusAndWait(doneButton), ui.LeftClick(doneButton))(ctx); err != nil {
		s.Fatal("Failed to find Done button and close the dialog: ", err)
	}

	// Verify the edit dialog is actually gone.
	if err := ui.WaitUntilGone(editDialog)(ctx); err != nil {
		s.Fatal("Failed to close the Edit dialog: ", err)
	}

	// Verify the disruptive Accel  did  not get added in the shortcut app and still has the default shortcut.
	if err := sc.VerifyShortcuts(ctx, ui, "Open/close calendar", sc.ShortcutKeys{Keys: shortcutName, Role: role.GenericContainer}); err != nil {
		s.Fatal("Lock screen added as shortcut hence failed: ", err)
	}

	// Verify the shortcut triggering the original action of locking screen.
	if err := kb.Accel(ctx, "search+l"); err != nil {
		s.Fatal("Failed to press search+l: ", err)
	}

	// Verify the Lock screen shortcut works and does the intended operation.
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		s.Fatalf("Failed to lock the screen: %v (last status %+v)", err, st)
	}

}
