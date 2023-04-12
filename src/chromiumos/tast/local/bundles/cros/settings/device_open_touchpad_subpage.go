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
		Func:         DeviceOpenTouchpadSubpage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open touchpad subpage and verify the built-in touchpad",
		Contacts: []string{
			"cros-peripherals@google.com",
			"wangdanny@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Touchpad
		BugComponent: "b:1131849",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Touchpad()),
	})
}

// DeviceOpenTouchpadSubpage opens the touchpad subpage and verifies
// if there is a built-in touchpad.
func DeviceOpenTouchpadSubpage(ctx context.Context, s *testing.State) {
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

	// Find Touchpad row and click it.
	touchpadRow := nodewith.Name("Touchpad").Role(role.GenericContainer)
	if err := ui.LeftClick(touchpadRow)(ctx); err != nil {
		s.Fatal("Failed to click touchpad row: ", err)
	}

	// Verify if built in touchpad shows up.
	touchpadHeading := nodewith.Name("Built-in Touchpad").Role(role.Heading)
	if err := ui.WaitUntilExists(touchpadHeading)(ctx); err != nil {
		s.Fatal("Failed to find built-in touchpad: ", err)
	}
}
