// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/ui"
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

	"go.chromium.org/tast/core/errors"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceSimulateRightClick,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the simulate right-click touchpad setting",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Touchpad
		BugComponent: "b:1131849",
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			ui.GaiaPoolDefaultVarName,
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Touchpad()),
	})
}

// DeviceSimulateRightClick opens the Touchpad settings subpage, updates the
// simulate right-click setting to  "alt + click", and then verifies that
// "alt + click" opens the right-click menu.
func DeviceSimulateRightClick(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.EnableFeatures("InputDeviceSettingsSplit", "AltClickAndSixPackCustomization"))
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	s.Log("Creating a virtual trackpad")
	tp, err := input.Trackpad(ctx)
	defer tp.Close(cleanupCtx)

	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open settings page: ", err)
	}

	// Find Touchpad row and click it.
	if err := ui.DoDefault(constants.TouchpadRow)(ctx); err != nil {
		s.Fatal("Failed to click touchpad row: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	rightClickDropdown := nodewith.Role(
		role.ComboBoxSelect)
	altClickOption := nodewith.Name("alt + click").Role(role.ListBoxOption)
	if err := uiauto.Combine("Choose alt + click option",
		ui.LeftClickUntil(rightClickDropdown, ui.WithTimeout(
			2*time.Second).WaitUntilExists(altClickOption)),
		ui.LeftClick(altClickOption),
		ui.WaitUntilExists(altClickOption),
	)(ctx); err != nil {
		s.Fatal("Failed to set the right-click setting to alt + click: ", err)
	}

	settings.Close(ctx)
	action.Sleep(time.Second)(ctx)
	if err := uiauto.Combine("Open right-click menu",
		kb.AccelPressAction("Alt"),
		action.Sleep(50*time.Millisecond),
		leftClickOnTouchpad(tp),
		action.Sleep(50*time.Millisecond),
		kb.AccelReleaseAction("Alt"),
		ui.WaitUntilExists(nodewith.NameContaining("Set wallpaper")),
	)(ctx); err != nil {
		s.Fatal("Failed to open right-click menu: ", err)
	}
}

func leftClickOnTouchpad(tp *input.TrackpadEventWriter) action.Action {
	return func(ctx context.Context) error {
		if err := tp.PressButton(input.BTN_LEFT); err != nil {
			return errors.Wrap(err, "failed to click on the touchpad")
		}
		return nil
	}
}
