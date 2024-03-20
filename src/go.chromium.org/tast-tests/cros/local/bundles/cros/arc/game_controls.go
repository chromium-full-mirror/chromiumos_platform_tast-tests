// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GameControls,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Installs the game controls test application, adds a new button, and verifies correctness",
		Contacts:     []string{"arc-gaming@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:arc", "group:arc-functional", "group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_arc_userdebug"},
		Fixture:      "arcBootedWithGameDashboard",
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
	})
}

func GameControls(ctx context.Context, s *testing.State) {
	gio.SetupTestApp(ctx, s, func(params gio.TestParams) error {
		// Start up UIAutomator.
		ui := uiauto.New(params.TestConn).WithTimeout(time.Minute).WithInterval(500 * time.Millisecond)

		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)

		// Create a new mouse.
		mouse, err := input.Mouse(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create mouse")
		}
		defer mouse.Close(ctx)

		gameControlsTile := nodewith.Name("Edit game controls").HasClass("GameDashboardMainMenuView::GameControlsDetailsRow")
		addButtonContainer := nodewith.Name("Create another control").HasClass("EditingList::AddContainerButton")
		menuDoneButton := nodewith.Name("Done").HasClass("DoneButton")
		listDoneButton := nodewith.Name("Done editing").HasClass("PillButton")

		const assignedKey = "r"
		return uiauto.Combine("find gaming input overlay UI elements",
			// Open game dashboard.
			kb.AccelAction("Search+g"),
			// Open game controls editing.
			ui.LeftClickUntil(gameControlsTile, ui.Gone(gameControlsTile)),
			// Add a new button.
			ui.LeftClickUntil(addButtonContainer, ui.Gone(addButtonContainer)),
			// Place the new button via the mouse.
			func(ctx context.Context) error { return mouse.Click() },
			// Begin to assign a new binding to the new button.
			ui.LeftClickUntilFocused(nodewith.Name("Key is missing. Tap on a keyboard key to assign").HasClass("EditLabel")),
			// Assign a new binding to the new button.
			kb.AccelAction(assignedKey),
			// Exit out of the button options menu.
			ui.LeftClickUntil(menuDoneButton, ui.Gone(menuDoneButton)),
			// Exit out of game controls editing.
			ui.LeftClickUntil(listDoneButton, ui.Gone(listDoneButton)),
			// Verify that the new button works as intended.
			gio.TapOverlayButton(kb, assignedKey, &params, gio.TopTap),
		)(ctx)
	})
}
