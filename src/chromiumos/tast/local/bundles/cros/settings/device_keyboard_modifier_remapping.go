// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	la "chromiumos/tast/local/chrome/uiauto/launcher"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/devicesettings/constants"
	"chromiumos/tast/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceKeyboardModifierRemapping,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify remapping functionality in remapping subpage",
		Contacts: []string{
			"cros-peripherals@google.com",
			"wangdanny@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
	})
}

// DeviceKeyboardModifierRemapping verifies the keyboard modifier remapping
// in the remapping subpage.
func DeviceKeyboardModifierRemapping(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

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

	// Find Keyboard row and click it.
	if err := ui.DoDefault(constants.KeyboardRow)(ctx); err != nil {
		s.Fatal("Failed to click keyboard row: ", err)
	}

	// Click customize keyboard keys row and verify if all the buttons show up.
	if err := ui.DoDefault(constants.CustomizeKeyboardKeys)(ctx); err != nil {
		s.Fatal("Failed to click Customize keyboard keys row: ", err)
	}

	resetKeysButton := nodewith.Name(
		"Reset keys").Role(role.Button)
	metaKeyRow := nodewith.NameRegex(
		regexp.MustCompile("(launcher)|(search)")).Role(role.GenericContainer)
	ctrlKeyRow := nodewith.Name(constants.Control).Role(role.GenericContainer)
	metaKey := nodewith.NameRegex(
		regexp.MustCompile("(launcher)|(search)")).Role(
		role.ComboBoxSelect).Ancestor(metaKeyRow)
	ctrlKey := nodewith.Name(constants.Control).Role(
		role.ComboBoxSelect).Ancestor(ctrlKeyRow)
	altKey := nodewith.Name(constants.Alt).Role(role.ComboBoxSelect)
	escapeKey := nodewith.Name(constants.Escape).Role(role.ComboBoxSelect)
	backspaceKey := nodewith.Name(constants.Backspace).Role(role.ComboBoxSelect)
	if err := uiauto.Combine("verify reset keys button and modifier remapping keys exist",
		ui.WaitUntilExists(resetKeysButton),
		ui.WaitUntilExists(metaKey),
		ui.WaitUntilExists(ctrlKey),
		ui.WaitUntilExists(altKey),
		ui.WaitUntilExists(escapeKey),
		ui.WaitUntilExists(backspaceKey),
	)(ctx); err != nil {
		s.Fatal("Failed to verify reset keys button or modifier remapping keys exist: ", err)
	}

	// Remap Launcher(Search) and Ctrl.
	launcherOrSearchOption := nodewith.NameRegex(
		regexp.MustCompile("(launcher)|(search)")).Role(role.ListBoxOption)
	controlOption := nodewith.Name(constants.Control).Role(role.ListBoxOption)

	if err := uiauto.Combine("choose control option",
		ui.LeftClickUntil(metaKey, ui.WithTimeout(
			2*time.Second).WaitUntilExists(controlOption)),
		ui.LeftClick(controlOption),
		ui.WaitUntilExists(controlOption),
	)(ctx); err != nil {
		s.Fatal("Failed to choose control option: ", err)
	}

	if err := uiauto.Combine("choose launcher(search) option",
		ui.LeftClickUntil(ctrlKey, ui.WithTimeout(
			2*time.Second).WaitUntilExists(launcherOrSearchOption)),
		ui.LeftClick(launcherOrSearchOption),
		ui.WaitUntilExists(launcherOrSearchOption),
	)(ctx); err != nil {
		s.Fatal("Failed to choose launcher option: ", err)
	}

	// Verify remapping functionality.
	// Open and close Launcher with Ctrl.
	launcher := nodewith.ClassName(la.ExpandedItemsClass).Visible().First()

	if err := uiauto.Combine(
		"Verify Ctrl can open launcher and then close it",
		// Open launcher with ctrl.
		kb.AccelAction("ctrl"),
		ui.WaitUntilExists(launcher),
		// Close launcher with ctrl.
		kb.AccelAction("ctrl"),
		ui.WaitUntilGone(launcher),
	)(ctx); err != nil {
		s.Fatal("Failed to open and close Launcher with Ctrl: ", err)
	}

	// Open Explore app with Launcher(Search) + /.
	if err := kb.Accel(ctx, "search+/"); err != nil {
		s.Fatal("Failed to press search+/: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Help.ID, time.Minute); err != nil {
		s.Fatal("Explore app did not appear in shelf after launch: ", err)
	}

	// Close Explore app.
	if err := apps.Close(ctx, tconn, apps.Help.ID); err != nil {
		s.Fatal("Failed to close Explore app: ", err)
	}

	// Verify reset keys button functionality.
	// Click reset keys button.
	if err := uiauto.Combine("Reset keys",
		ui.DoDefault(resetKeysButton),
		ui.WaitUntilExists(metaKey),
		ui.WaitUntilExists(ctrlKey),
	)(ctx); err != nil {
		s.Fatal("Failed to reset keys: ", err)
	}

	// Open and close Launcher with Launcher(Search).
	if err := uiauto.Combine(
		"Verify Launcher(Search) can open launcher and then close it",
		// Open launcher with Launcher(Search).
		kb.AccelAction("search"),
		ui.WaitUntilExists(launcher),
		// Close launcher with Launcher(Search).
		kb.AccelAction("search"),
		ui.WaitUntilGone(launcher),
	)(ctx); err != nil {
		s.Fatal("Failed to open and close Launcher with Launcher(Search): ", err)
	}

	// Open Explore app with Ctrl + /.
	if err := kb.Accel(ctx, "ctrl+/"); err != nil {
		s.Fatal("Failed to press ctrl+/: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Help.ID, time.Minute); err != nil {
		s.Fatal("Explore app did not appear in shelf after launch: ", err)
	}

	// Close Explore app.
	if err := apps.Close(ctx, tconn, apps.Help.ID); err != nil {
		s.Fatal("Failed to close Explore app: ", err)
	}
}
