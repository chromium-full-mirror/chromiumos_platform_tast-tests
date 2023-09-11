// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/devicesettings"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceKeyboardFkeys,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Modifying a F11/F12 shortcut works",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Timeout:      2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

// DeviceKeyboardFkeys opens the remap keys subpage, changes the shortcuts
// used to trigger the F11/F12 key actions and verifies that they work
// correctly.
func DeviceKeyboardFkeys(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.EnableFeatures("InputDeviceSettingsSplit", "AltClickAndSixPackCustomization", "SupportF11AndF12KeyShortcuts"))
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	settings, err := ossettings.LaunchAtPage(ctx, tconn, nodewith.Name("Device").Role(role.Link))
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)
	topRowKeyButton := nodewith.Name("Treat top-row keys as function keys")
	if err := uiauto.Combine("open customize keyboard keys page",
		ui.DoDefault(constants.KeyboardRow),
		ui.WaitUntilExists(topRowKeyButton),
		ui.DoDefault(topRowKeyButton),
		ui.WaitUntilExists(topRowKeyButton.Attribute("checked", "true")),
		ui.DoDefault(constants.CustomizeKeyboardKeys),
	)(ctx); err != nil {
		s.Fatal("Failed to open customize keyboard keys page: ", err)
	}

	mew, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to get mouse: ", err)
	}
	defer mew.Close(ctx)

	const (
		maxNumSelectRetries = 4
		numScrolls          = 10
	)

	for i := 0; i < maxNumSelectRetries; i++ {
		for j := 0; j < numScrolls; j++ {
			if err := mew.ScrollDown(); err != nil {
				s.Fatal("Failed to scroll down: ", err)
			}
		}
	}

	if err := devicesettings.Remap(ctx, ui, constants.F11, constants.F11ShiftShortcut); err != nil {
		s.Fatal("Failed to remap F11: ", err)
	}

	if err := devicesettings.Remap(ctx, ui, constants.F12, constants.F12AltShortcut); err != nil {
		s.Fatal("Failed to remap F12: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := kb.Accel(ctx, "Shift+F1"); err != nil {
		s.Fatal("Failed to get press Shift+F1: ", err)
	}

	if err := ash.WaitForFullScreen(ctx, tconn); err != nil {
		s.Fatal("Failed waiting for full screen: ", err)
	}

	if err := kb.Accel(ctx, "Alt+F2"); err != nil {
		s.Fatal("Failed to get press Alt+F2: ", err)
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(nodewith.NameContaining("Console").First())(ctx); err != nil {
		s.Fatal("Failed to find developer console: ", err)
	}
}

// selectCollectionNode waits for an element on the page to exist/become
// visible and then triggers the default action on the node e.g. left click on
// a button.
func selectCollectionNode(ui *uiauto.Context, collectionNode *nodewith.Finder) uiauto.Action {
	return uiauto.Combine("select collection node",
		ui.WaitUntilExists(collectionNode),
		ui.MakeVisible(collectionNode),
		ui.DoDefault(collectionNode),
	)
}
