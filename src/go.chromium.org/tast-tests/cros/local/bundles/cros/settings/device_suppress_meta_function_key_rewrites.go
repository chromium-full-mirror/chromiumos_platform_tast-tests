// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceSuppressMetaFunctionKeyRewrites,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test if user can suppress meta + function key rewrites",
		Contacts: []string{
			"cros-peripherals@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Keyboard()),
	})
}

// DeviceSuppressMetaFunctionKeyRewrites tests if the functionality of
// using meta key to change the behavior of Chromebook top row keys can be suppressed.
func DeviceSuppressMetaFunctionKeyRewrites(ctx context.Context, s *testing.State) {
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

	// Set up virtual external keyboard.
	vk, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer vk.Close(cleanupCtx)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Verify if keyboard row appears and click it.
	if err := uiauto.Combine("Verify if the keyboard row appears and click it",
		ui.WaitUntilExists(constants.KeyboardRow),
		ui.DoDefault(constants.KeyboardRow),
	)(ctx); err != nil {
		s.Fatal("Failed to find the keyboard row and click it: ", err)
	}

	// Verify if virtual keyboard is created.
	vkHeading := nodewith.NameContaining("Tast virtual keyboard").Role(role.Heading)
	if err := ui.WaitUntilExists(vkHeading)(ctx); err != nil {
		s.Fatal("Failed to find Tast virtual keyboard: ", err)
	}

	// Verify pressing launcher/search + top row back button isn't suppressed
	// and will go back to device page.
	topRow, err := input.KeyboardTopRowLayout(ctx, vk)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard top row: ", err)
	}
	if err := vk.Accel(ctx, "search+"+topRow.BrowserBack); err != nil {
		s.Fatal("Failed to press launcher/search + top row back button: ", err)
	}

	// Verify if keyboard row appears and click it.
	if err := uiauto.Combine("Verify if the keyboard row appears and click it",
		ui.WaitUntilExists(constants.KeyboardRow),
		ui.DoDefault(constants.KeyboardRow),
	)(ctx); err != nil {
		s.Fatal("Failed to find the keyboard row and click it: ", err)
	}

	// Turn off the toggle to suppress meta + function key rewrites.
	topRowKeyButton := nodewith.NameContaining("change the behavior of function keys").First()
	if err := uiauto.Combine("Verify if the toggle is turned off",
		ui.WaitUntilExists(topRowKeyButton),
		ui.DoDefault(topRowKeyButton),
		ui.WaitUntilExists(topRowKeyButton.Attribute("checked", "false")),
	)(ctx); err != nil {
		s.Fatal("Failed to turn off the toggle: ", err)
	}

	// Verify pressing search + top row back button is suppressed
	// and won't go back to device page.
	if err := vk.Accel(ctx, "search+"+topRow.BrowserBack); err != nil {
		s.Fatal("Failed to press launcher/search + top row back button: ", err)
	}

	if err := ui.WaitUntilExists(vkHeading)(ctx); err != nil {
		s.Fatal("Failed to find Tast virtual keyboard: ", err)
	}
}
