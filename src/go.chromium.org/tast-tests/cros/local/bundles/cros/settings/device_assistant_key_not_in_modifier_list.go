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
		HardwareDeps: hwdep.D(hwdep.NoAssistantKey(), hwdep.InternalKeyboard()),
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

	// Click customize keyboard keys row and verify if all the buttons show up.
	if err := ui.DoDefault(constants.CustomizeKeyboardKeys)(ctx); err != nil {
		s.Fatal("Failed to click Customize keyboard keys row: ", err)
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
