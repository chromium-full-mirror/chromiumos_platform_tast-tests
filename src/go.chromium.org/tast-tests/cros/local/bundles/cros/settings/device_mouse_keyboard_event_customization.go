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
	ds "go.chromium.org/tast-tests/cros/local/devicesettings"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceMouseKeyboardEventCustomization,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test mouse key customization to a keyboard action in device settings",
		Contacts: []string{
			"cros-peripherals@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > Fundamentals > Peripherals > Mouse
		BugComponent: "b:1131847",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
	})
}

// DeviceMouseKeyboardEventCustomization tests mouse scroll acceleration
// enablement and scrolling speed slider.
func DeviceMouseKeyboardEventCustomization(ctx context.Context, s *testing.State) {
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

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)
	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Navigate to mouse customization page for the attached virtual mouse.
	if err := ds.NavigateMouseCustomization(ctx, ui, constants.MouseLabel); err != nil {
		s.Fatal("Failed to navigate to mouse customization page: ", err)
	}

	// Click mouse to trigger keyboard event.
	if err := mouse.KeyboardActionClick(input.KEY_S); err != nil {
		s.Fatal("Failed to click with the Keyboard action: ", err)
	}

	// Verify now the clicked button is detected and displayed in UI.
	mouseButton := nodewith.Name(constants.OtherButton).Role(role.StaticText)
	if err := ui.WaitUntilExists(mouseButton)(ctx); err != nil {
		s.Fatal("Failed to find the button row in the page button customization subpage: ", err)
	}

	// Verify the dropdown also is displayed along with the button.
	actionDropdown := nodewith.HasClass("md-select").First()
	if err := ui.WaitUntilExists(actionDropdown)(ctx); err != nil {
		s.Fatal("Failed to find the button customization dropdown: ", err)
	}

	// Remapping the button to show Overview.
	if err := uiauto.Combine("Remapping action from drop down",
		dropdown.SelectCustomizePeripheralButtonsDropdown(tconn, actionDropdown, "Overview"),
	)(ctx); err != nil {
		s.Fatal("Failed to remap action from drop down for the button: ", err)
	}

	// Closing the Setting app window.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w and close the settings app: ", err)
	}

	// Clicking newly mapped mouse button.
	if err := mouse.KeyboardActionClick(input.KEY_S); err != nil {
		s.Fatal("Failed to click with the mouse button again to trigger mapped action: ", err)
	}

	// Verifying the keyboard event is triggered by mouse click to open overview ui.
	if err := ui.WaitUntilExists(constants.OverviewMode)(ctx); err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to trigger Overview by clicking: ", err)
	}

	// Disconnecting the mouse.
	mouse.Close(cleanupCtx)

	// Relaunching settings app
	settings, err = ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page again: ", err)
	}

	// Reconnecting the mouse.
	mouse, err = input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create mouse: ", err)
	}

	// Navigate to mouse customization page for the attached virtual mouse.
	if err := ds.NavigateMouseCustomization(ctx, ui, constants.MouseLabel); err != nil {
		s.Fatal("Failed to navigate to mouse customization page: ", err)
	}

	// Closing settings app.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w: ", err)
	}

	// Clicking middle mouse button again to check if it triggers same
	// action of opening overview ui.
	if err := mouse.KeyboardActionClick(input.KEY_S); err != nil {
		s.Fatal("Failed to click with mouse button as keyboard event: ", err)
	}

	// Verifying if Overview mode is on.
	if err := ui.WaitUntilExists(constants.OverviewMode)(ctx); err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to trigger Overview by clicking: ", err)
	}
}
