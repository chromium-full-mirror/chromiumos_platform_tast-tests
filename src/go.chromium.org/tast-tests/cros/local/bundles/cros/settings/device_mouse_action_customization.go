// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	ds "go.chromium.org/tast-tests/cros/local/devicesettings"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceMouseActionCustomization,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test mouse key customization in device settings",
		Contacts: []string{
			"cros-peripherals@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Mouse
		BugComponent: "b:241965717",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

// DeviceMouseActionCustomization tests the mapping of different action to a mouse button.
func DeviceMouseActionCustomization(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 6*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	// Set up mouse.
	mouse, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create mouse: ", err)
	}

	defer mouse.Close(ctx)

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Navigating to mouse button customization page again.
	if err := ds.NavigateMouseCustomization(ctx, ui, constants.MouseLabel); err != nil {
		s.Fatal("Failed to navigate to mouse customization page: ", err)
	}

	// Click the middle button for the mouse.
	if err := mouse.MiddleClick(); err != nil {
		s.Fatal("Failed to click with the middle button: ", err)
	}

	// Verify the dropdown also is displayed along with the button.
	actionDropdown := nodewith.HasClass("md-select").First()
	testing.ContextLog(ctx, "Combobox clicked")

	// Remapping the button to Screenshot action.
	if err := uiauto.Combine("Remapping action from drop down",
		dropdown.SelectCustomizePeripheralButtonsDropdown(tconn, actionDropdown, "Screenshot"),
	)(ctx); err != nil {
		s.Fatal("Failed to remap action from drop down for the button: ", err)
	}

	// Closing the Setting app window.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w to close the app: ", err)
	}

	// Verifying if the middle button performs the action assigned of taking screenshot.
	if err := mouse.MiddleClick(); err != nil {
		s.Fatal("Failed to click with the middle button again to trigger mapped action: ", err)
	}

	// Verifying if Emoji picker is open.
	if err := ui.WaitUntilExists(nodewith.Name("Drag to select an area to capture").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Failed to open screenshot mode immediately after performing middle button click: ", err)
	}

	closeButton := nodewith.Name("Close").Role(role.Button)
	if err := ui.WithTimeout(2 * time.Minute).LeftClick(closeButton)(ctx); err != nil {
		s.Fatal(err, "Failed to close the screen shot mode")
	}

	// Disconnecting the mouse.
	mouse.Close(ctx)

	// Relaunching settings app.
	settings, err = ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page at the end of test: ", err)
	}

	// Reconnecting the mouse.
	mouse, err = input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create mouse: ", err)
	}

	defer mouse.Close(ctx)

	// Navigating to mouse button customization page again.
	if err := ds.NavigateMouseCustomization(ctx, ui, "Tast virtual mouse"); err != nil {
		s.Fatal("Failed to navigate to mouse customization page after reconnecting the mouse: ", err)
	}

	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w : ", err)
	}

	// Clicking middle mouse button again to check if it triggers same
	// action of taking screenshot.
	if err := mouse.MiddleClick(); err != nil {
		s.Fatal("Failed to click with the middle button again to trigger mapped action: ", err)
	}

	// Verifying if Emoji picker is open.
	if err := ui.WaitUntilExists(nodewith.Name("Drag to select an area to capture").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Failed to Open Shortcut mode after assigning to middle button click for mouse post disconnection: ", err)
	}
}
