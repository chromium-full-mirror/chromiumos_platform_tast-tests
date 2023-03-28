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
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceOpenKeyboardSubpage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open keyboard subpage and verify the built-in keyboard",
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

// DeviceOpenKeyboardSubpage opens the keyboard subpage and verifies
// if there is a built-in keyboard.
func DeviceOpenKeyboardSubpage(ctx context.Context, s *testing.State) {
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
	keyboardRow := nodewith.Name("Keyboard").Role(role.Link)
	if err := ui.DoDefault(keyboardRow)(ctx); err != nil {
		s.Fatal("Failed to click keyboard row: ", err)
	}

	// Verify if built in keyboard shows up.
	keyboardHeading := nodewith.Name("Built-in Keyboard").Role(role.Heading)
	if err := ui.WaitUntilExists(keyboardHeading)(ctx); err != nil {
		s.Fatal("Failed to find built-in keyboard: ", err)
	}
}
