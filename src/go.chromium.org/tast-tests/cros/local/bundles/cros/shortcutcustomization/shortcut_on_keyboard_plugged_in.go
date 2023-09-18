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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	sc "go.chromium.org/tast-tests/cros/local/chrome/uiauto/shortcutcustomization"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShortcutOnKeyboardPluggedIn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Shortcuts change when an external keyboard is plugged in",
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

// ShortcutOnKeyboardPluggedIn verifies shortcut will change when an external keyboard is plugged in.
func ShortcutOnKeyboardPluggedIn(ctx context.Context, s *testing.State) {
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

	// Launch shortcut customization app.
	shortcutCustomizationRootNode, err := sc.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch shortcut customization app: ", err)
	}

	// Verify shortcut customization app is launched.
	if err := sc.VerifyShortcutCustomizationIsLaunched(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to verify that the Shortcut Customization app is launched: ", err)
	}

	// Select "Device" from the side nav.
	deviceCategory := nodewith.Name("Device").Role(role.StaticText).Ancestor(shortcutCustomizationRootNode)
	if err := ui.DoDefault(deviceCategory)(ctx); err != nil {
		s.Fatal("Failed to click Device category: ", err)
	}

	// Verify “Turn volume up” shortcut and observe “[volume icon]” as the shortcut key.
	if err := sc.VerifyShortcuts(ctx, ui, "Turn volume up", sc.ShortcutKeys{Keys: "volume-up", Role: role.GenericContainer}); err != nil {
		s.Fatal("Failed to observe [volume up] icon as shortcut keys for Turn volume up: ", err)
	}

	// Connect to a keyboard, and shortcut should be updated to add Fkeys as shortcut keys.
	vkb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a virtual keyboard: ", err)
	}
	defer vkb.Close(cleanupCtx)

	// Verify both “[volume icon]” and "F10" are shortcut keys for for "Turn volume up".
	turnVolumeUpShortcuts := []struct {
		description string
		keys        sc.ShortcutKeys
	}{
		{"Turn volume up", sc.ShortcutKeys{Keys: "volume-up", Role: role.GenericContainer}},
		{"Turn volume up", sc.ShortcutKeys{Keys: "F10", Role: role.StaticText}},
	}
	for _, shortcut := range turnVolumeUpShortcuts {
		if err := sc.VerifyShortcuts(ctx, ui, shortcut.description, shortcut.keys); err != nil {
			s.Fatal("Failed to observe [volume up] icon and F10 as shortcut keys for Turn volume up: ", err)
		}
	}
}
