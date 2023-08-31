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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceSwapPrimaryMouseButton,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test if primary mouse button can be swapped to right button",
		Contacts: []string{
			"cros-peripherals@google.com",
			"wangdanny@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Mouse
		BugComponent: "b:1131847",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
	})
}

// DeviceSwapPrimaryMouseButton tests if the primary mouse button
// can be swapped from left button to right button.
func DeviceSwapPrimaryMouseButton(ctx context.Context, s *testing.State) {
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

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Set up mouse.
	mouse, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create mouse: ", err)
	}
	defer mouse.Close(ctx)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Find Mouse row and click it.
	if err := ui.DoDefault(constants.MouseRow)(ctx); err != nil {
		s.Fatal("Failed to click mouse row: ", err)
	}

	// Swap primary mouse button to Right button.
	mouseComboBoxSelect := nodewith.Role(role.ComboBoxSelect)
	rightButtonOption := nodewith.NameContaining("Right button").Role(role.ListBoxOption)

	if err := uiauto.Combine("choose right button option",
		ui.LeftClickUntil(mouseComboBoxSelect, ui.WithTimeout(
			2*time.Second).WaitUntilExists(rightButtonOption)),
		ui.LeftClick(rightButtonOption),
		ui.WaitUntilExists(rightButtonOption),
	)(ctx); err != nil {
		s.Fatal("Failed to choose right option: ", err)
	}

	// Test if primary mouse button is right button.
	accelerationToggleButton := nodewith.Role(role.ToggleButton).First()
	if err := ui.WaitUntilExists(accelerationToggleButton)(ctx); err != nil {
		s.Fatal("Failed to find acceleration toggle button: ", err)
	}
	if err := mouse.RightClick(); err != nil {
		s.Fatal("Failed to click acceleration row: ", err)
	}
	if err := ui.WaitUntilExists(accelerationToggleButton.Attribute("checked", "false"))(ctx); err != nil {
		s.Fatal("Failed to change acceleration toggle button value: ", err)
	}
}
