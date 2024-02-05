// Copyright 2024 The ChromiumOS Authors
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
	ds "go.chromium.org/tast-tests/cros/local/devicesettings"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceMouseButtonRenaming,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test mouse button renaming in device settings",
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

// DeviceMouseButtonRenaming tests mouse scroll acceleration enablement and scrolling speed slider.
func DeviceMouseButtonRenaming(ctx context.Context, s *testing.State) {
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
	defer kb.Close(ctx)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Navigate to mouse customization page for the attached
	// virtual mouse.
	if err := ds.NavigateMouseCustomization(ctx, ui, constants.MouseLabel); err != nil {
		s.Fatal("Failed to navigate to mouse customization page: ", err)
	}

	// Click the middle button for the mouse.
	if err := mouse.MiddleClick(); err != nil {
		s.Fatal("Failed to click with the middle button: ", err)
	}

	// Click mouse to trigger keyboard event.
	if err := mouse.KeyboardActionClick(input.KEY_S); err != nil {
		s.Fatal("Failed to click with the Keyboard action: ", err)
	}

	// Verify now the clicked button is detected and displayed in UI.
	mouseButton1 := nodewith.Name(constants.MiddleButton).Role(role.StaticText)
	if err := ui.WaitUntilExists(mouseButton1)(ctx); err != nil {
		s.Log("Failed to find the button: ", err)
	}

	// Clicking the edit button te rename the button.
	if err := ui.LeftClick(nodewith.ClassName(constants.EditButton).Role(role.Button).First())(ctx); err != nil {
		s.Fatal("Failed to Find or click edit icon: ", err)
	}

	// Name of the node for the button name.
	textBoxName := "Change button name"

	// Clear the text box for button name
	if err := ds.ClearTextArea(ctx, ui, kb, textBoxName); err != nil {
		s.Fatal("Failed to clear text box area for Middle Button: ", err)
	}

	// Type the new name for the button.
	if err := kb.Type(ctx, constants.RenamedButton1); err != nil {
		s.Fatal("Failed to type /rename button: ", err)
	}

	if err := ui.LeftClick(nodewith.Name(constants.SaveButton).Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to Find or click Save post renaming: ", err)
	}

	if err := ui.WaitUntilExists(nodewith.Name(constants.RenamedButton1).Role(role.StaticText))(ctx); err != nil {
		s.Log("Failed to find the renamed button: ", err)
	}

	// Verifying for Other button 1.
	mouseButton2 := nodewith.Name(constants.OtherButton).Role(role.StaticText)
	if err := ui.WaitUntilExists(mouseButton2)(ctx); err != nil {
		s.Log("Failed to find the Other Button 1: ", err)
	}

	// Clicking edit for Other button 1.
	if err := ui.LeftClick(nodewith.ClassName(constants.EditButton).Role(role.Button).Nth(1))(ctx); err != nil {
		s.Fatal("Failed to Find or click edit icon: ", err)
	}

	// Clear the text box for button name.
	if err := ds.ClearTextArea(ctx, ui, kb, textBoxName); err != nil {
		s.Fatal("Failed to clear text box area for Other Button 1: ", err)
	}

	// Type the new name for the button.
	if err := kb.Type(ctx, constants.RenamedButton2); err != nil {
		s.Fatal("Failed to type/rename button: ", err)
	}

	if err := ui.LeftClick(nodewith.Name(constants.SaveButton).Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to Find or click Save post renaming: ", err)
	}

	// Closing the Setting app window.
	if err := kb.Accel(ctx, "ctrl+shift+w"); err != nil {
		s.Fatal("Failed to press ctrl+shift+w: ", err)
	}

	// Disconnecting the mouse.
	mouse.Close(ctx)

	// Relaunching settings app.
	settings, err = ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page again: ", err)
	}

	// Reconnecting the mouse.
	mouse, err = input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create mouse: ", err)
	}

	//Navigating to mouse button customization page again.
	if err := ds.NavigateMouseCustomization(ctx, ui, constants.MouseLabel); err != nil {
		s.Fatal("Failed to navigate to mouse customization page: ", err)
	}

	// Verifying if the renamed button appears again.
	if err := uiauto.Combine("Checking if the buttons retain the renamed names after reconnecting mouse ",
		ui.WaitUntilExists(nodewith.Name(constants.RenamedButton1).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name(constants.RenamedButton2).Role(role.StaticText)),
	)(ctx); err != nil {
		s.Log("Failed to find the renamed button: ", err)
	}
	s.Log("Buttons renamed and retained after disconnecting mouse")
}
