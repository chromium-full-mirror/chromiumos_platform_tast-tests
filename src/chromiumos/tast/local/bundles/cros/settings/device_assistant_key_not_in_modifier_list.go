// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceAssistantKeyNotInModifierList,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify assistant key does not exist in remapping subpage",
		Contacts: []string{
			"cros-peripherals@google.com",
			"yyhyyh@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.NoAssistantKey()),
	})
}

// DeviceAssistantKeyNotInModifierList verifies the keyboard modifier remapping
// doesn't display the assistant key row when the device doesn't support
// assistant key.
func DeviceAssistantKeyNotInModifierList(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Open OS Settings app.
	if err := apps.Launch(ctx, tconn, apps.Settings.ID); err != nil {
		s.Fatal("Failed to launch Settings app: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		s.Fatal("Settings app did not appear in shelf after launch: ", err)
	}

	// Find Device row and click it.
	deviceRow := nodewith.Name("Device").Role(role.Link)
	if err := ui.DoDefault(deviceRow)(ctx); err != nil {
		s.Fatal("Failed to click device row: ", err)
	}

	// Find Keyboard row and click it.
	keyboardRow := nodewith.Name("Keyboard").Role(role.GenericContainer)
	if err := ui.DoDefault(keyboardRow)(ctx); err != nil {
		s.Fatal("Failed to click keyboard row: ", err)
	}

	// Click remap keyboard keys row and verify if all the buttons show up.
	remapKeyboardKeys := nodewith.Name(
		"Built-in Keyboard Customize keyboard keys").Role(role.GenericContainer)
	if err := ui.DoDefault(remapKeyboardKeys)(ctx); err != nil {
		s.Fatal("Failed to click Built-in Keyboard Customize keyboard keys row: ", err)
	}

	assistantKeyRow := nodewith.Name("assistant").Role(role.GenericContainer)
	ctrlKeyRow := nodewith.Name("ctrl").Role(role.GenericContainer)
	assistantKey := nodewith.Role(
		role.ComboBoxSelect).Ancestor(assistantKeyRow)
	ctrlKey := nodewith.Name("ctrl").Role(
		role.ComboBoxSelect).Ancestor(ctrlKeyRow)
	if err := ui.WaitUntilExists(ctrlKey)(ctx); err != nil {
		s.Fatal("Failed to verify ctrl key exists: ", err)
	}
	if err := ui.WaitUntilGone(assistantKey)(ctx); err != nil {
		s.Fatal("Failed to verify assistant key does not exist: ", err)
	}
}
