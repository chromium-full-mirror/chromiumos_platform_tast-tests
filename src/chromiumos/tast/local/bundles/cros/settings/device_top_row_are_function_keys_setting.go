// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceTopRowAreFunctionKeysSetting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the top row back button functionality with settings change",
		Contacts: []string{
			"cros-peripherals@google.com",
			"wangdanny@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
	})
}

// DeviceTopRowAreFunctionKeysSetting verifies the top row back button
// functionality with settings change. If the toggle is off, pressing
// the back button should go back to the previous page and pressing
// the search+back should open the Explore app. If the toggle is on,
// pressing the back button should open the Explore app and pressing
// search+back should go back to the previous page.
func DeviceTopRowAreFunctionKeysSetting(ctx context.Context, s *testing.State) {
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

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	defer kb.Close()

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
	if err := ui.LeftClick(keyboardRow)(ctx); err != nil {
		s.Fatal("Failed to click keyboard row: ", err)
	}

	// Verify if pressing top row back button
	// navigates back to the device page when the toggle is off.
	if err := kb.Accel(ctx, topRow.BrowserBack); err != nil {
		s.Fatal("Failed to press top row back button: ", err)
	}
	if err := ui.WaitUntilExists(keyboardRow)(ctx); err != nil {
		s.Fatal("Failed to wait until keyboard row exists: ", err)
	}

	// Verify if the Explore app is opened
	// when pressing search + top row back button.
	if err := kb.Accel(ctx, "search+"+topRow.BrowserBack); err != nil {
		s.Fatal("Failed to open Explore app with search + top row back button: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Help.ID, time.Minute); err != nil {
		s.Fatal("Explore app did not appear in shelf after launch: ", err)
	}

	// Close Explore app.
	if err := apps.Close(ctx, tconn, apps.Help.ID); err != nil {
		s.Fatal("Failed to close Explore app: ", err)
	}

	// Find Keyboard row and click it.
	if err := ui.DoDefault(keyboardRow)(ctx); err != nil {
		s.Fatal("Failed to click keyboard row: ", err)
	}

	// Turn on the toggle.
	topRowKeyButton := nodewith.Name("Treat top-row keys as function keys")
	if err := uiauto.Combine("Verify if the toggle is turned on",
		ui.WaitUntilExists(topRowKeyButton),
		ui.DoDefault(topRowKeyButton),
		ui.WaitUntilExists(topRowKeyButton.Attribute("checked", "true")),
	)(ctx); err != nil {
		s.Fatal("Failed to test disruptive keys: ", err)
	}

	// Verify if the Explore app is opened when pressing top row back button.
	if err := kb.Accel(ctx, topRow.BrowserBack); err != nil {
		s.Fatal("Failed to open Explore app with top row back button: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Help.ID, time.Minute); err != nil {
		s.Fatal("Explore app did not appear in shelf after launch: ", err)
	}

	// Close Explore app.
	if err := apps.Close(ctx, tconn, apps.Help.ID); err != nil {
		s.Fatal("Failed to close Explore app: ", err)
	}

	// Verify if pressing search + top row back button navigates back to
	// the device page when the toggle is on.
	if err := kb.Accel(ctx, "search+"+topRow.BrowserBack); err != nil {
		s.Fatal("Failed to press search + top row back button: ", err)
	}
	if err := ui.WaitUntilExists(keyboardRow)(ctx); err != nil {
		s.Fatal("Failed to wait until keyboard row exists: ", err)
	}
}
