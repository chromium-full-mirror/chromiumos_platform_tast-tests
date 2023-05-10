// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/devicesettings/constants"
	"chromiumos/tast/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceAddNewKeyboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test to add a new keyboard",
		Contacts: []string{
			"cros-peripherals@google.com",
			"wangdanny@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
	})
}

// DeviceAddNewKeyboard tests if a new keyboard appear in the keyboard subpage
// when adding a new keyboard.
func DeviceAddNewKeyboard(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Set up a new chrome since we are changing settings in the test.
	s.Log("Setting up chrome")
	cr, err := chrome.New(ctx, chrome.EnableFeatures("InputDeviceSettingsSplit"))
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

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Set up virtual keyboard.
	vk, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create virtual keyboard: ", err)
	}
	defer vk.Close(cleanupCtx)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Find Keyboard row and click it.
	if err := ui.DoDefault(constants.KeyboardRow)(ctx); err != nil {
		s.Fatal("Failed to click keyboard row: ", err)
	}

	// Verify if tast virtual keyboard shows up.
	vkHeading := nodewith.NameContaining("Tast virtual keyboard").Role(role.Heading)
	if err := ui.WaitUntilExists(vkHeading)(ctx); err != nil {
		s.Fatal("Failed to find Tast virtual keyboard: ", err)
	}
}
