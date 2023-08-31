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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	sc "go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShortcutOnInputLanguageChange,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Shortcuts change when the keyboard language changes",
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

// ShortcutOnInputLanguageChange verifies changing input language will change the shortcuts.
func ShortcutOnInputLanguageChange(ctx context.Context, s *testing.State) {
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

	ui := uiauto.New(tconn)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Open Inputs and keyboards page under "Languages and inputs".
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osLanguages/input", ui.Exists(nodewith.Name("Add input methods").Role(role.Button))); err != nil {
		s.Fatal("Failed to open inputs page: ", err)
	}

	targetLanguageFullName := "German (Germany)"
	targetLanguageCheckBox := nodewith.Name(targetLanguageFullName).Role(role.CheckBox)
	AddInputMethodsButton := nodewith.Name("Add input methods").Role(role.Button)
	searchLanguages := nodewith.Name("Search by language or input name").Role(role.SearchBox)

	// Click "Add input methods" to open languages list and wait for the search input to appear.
	if err := uiauto.Combine("open languages list",
		ui.LeftClick(AddInputMethodsButton),
		ui.WaitUntilExists(searchLanguages),
	)(ctx); err != nil {
		s.Fatal("Failed to open languages list: ", err)
	}

	// Type "German" into the search input to filter the language list.
	if err := uiauto.Combine("search for German language",
		ui.LeftClick(searchLanguages),
	)(ctx); err != nil {
		s.Fatal("Failed to click on search languages input: ", err)
	}

	// Type the search query using the keyboard.
	if err := kb.Type(ctx, "German"); err != nil {
		s.Fatal("Failed to type German: ", err)
	}

	// Wait for the "German (Germany)" checkbox to appear.
	if err := ui.WaitUntilExists(targetLanguageCheckBox)(ctx); err != nil {
		s.Fatal("Failed to find German language checkbox: ", err)
	}

	// Add German input methods.
	if err := uiauto.Combine("add German input methods",
		ui.FocusAndWait(targetLanguageCheckBox),
		ui.LeftClick(targetLanguageCheckBox),
		ui.LeftClick(nodewith.Name("Add").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to add German input methods: ", err)
	}

	// Launch shortcut customization app.
	shortcutCustomizationRootNode, err := sc.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch shortcut customization app: ", err)
	}
	// Verify shortcut customization app is launched.
	if err := sc.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Verify two shortcuts:
	// 1. Open Help in Explore app: "ctrl /" for US input and "ctrl -" for German input.
	// 2. Pin window to left: "alt [" for US input and "alt ü" for German input.
	// Verify the US input shortcut for "Open Help in Explore app" is "ctrl /"
	if err := sc.VerifyShortcuts(ctx, ui, "Open \"Help\" in Explore app", sc.ShortcutKeys{Keys: "ctrl /", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find ctrl+/ for us input for 'Open Explore app' shortcut: ", err)
	}
	// Go to "Windows and desks" from the side nav.
	WindowAndDesksCategory := nodewith.Name("Windows and desks").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(WindowAndDesksCategory)(ctx); err != nil {
		s.Fatal("Failed to click Windows and desks category: ", err)
	}
	// Verify the US input shortcut for "Pin window to left" is "alt ["
	if err := sc.VerifyShortcuts(ctx, ui, "Pin window to left", sc.ShortcutKeys{Keys: "alt [", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find alt+[ for us input for 'Pin window to left' shortcut: ", err)
	}

	// Press Ctrl-Space to switch to German input and reload app
	if err := kb.Accel(ctx, "Ctrl+Space"); err != nil {
		s.Fatal("Failed to send Ctrl-Space: ", err)
	}
	if err := kb.Accel(ctx, "Ctrl+R"); err != nil {
		s.Fatal("Failed to send Ctrl-R: ", err)
	}

	// Verify the German input shortcut for "Open Help in Explore app" is "ctrl -"
	if err := sc.VerifyShortcuts(ctx, ui, "Open \"Help\" in Explore app", sc.ShortcutKeys{Keys: "ctrl -", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find 'ctrl -' for German input for 'Open Explore app' shortcut: ", err)
	}
	// Go to "Windows and desks" from the side nav.
	if err := ui.DoDefault(nodewith.Name("Windows and desks").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Failed to click Windows and desks category: ", err)
	}
	// Verify the German input shortcut for "Pin window to left" is "alt ü"
	if err := sc.VerifyShortcuts(ctx, ui, "Pin window to left", sc.ShortcutKeys{Keys: "alt ü", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to find alt+ü for German input for 'Pin window to left' shortcut: ", err)
	}
}
