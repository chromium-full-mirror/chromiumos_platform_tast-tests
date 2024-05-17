// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputOverlayEditor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test for gaming input overlay key mapping editor correctness",
		Contacts:     []string{"arc-app-dev@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithGameDashboard",
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container_r"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
	})
}

func InputOverlayEditor(ctx context.Context, s *testing.State) {
	gio.SetupTestApp(ctx, s, func(params gio.TestParams) error {
		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, params.TestConn)
		// Start up UIAutomator.
		ui := uiauto.New(params.TestConn).WithTimeout(time.Minute)

		gameControlsEdit := nodewith.Name("Edit game controls").HasClass("GameDashboardMainMenuView::GameControlsDetailsRow")
		topTapListItem := nodewith.Name("Selected key is space. Tap on the button to edit the control").HasClass("ActionViewListItem")
		topTapButtonOptionsEditLabel := nodewith.Name("Selected key is space. Tap on another keyboard key to replace").HasClass("EditLabel")
		topMoveEditingListEditLabel := nodewith.Name("Selected key is w for up. Tap on another keyboard key to replace").HasClass("EditLabel")
		botTapEditingListEditLabel := nodewith.Name("Selected key is n. Tap on another keyboard key to replace").HasClass("EditLabel")
		illegalMappingDescription := nodewith.Name("The following keys aren’t supported: Tab, Shift, Control, Escape, Caps lock, Volume").HasClass("ImageView")
		buttonOptionsDone := nodewith.Name("Done").HasClass("DoneButton")
		editingListDone := nodewith.Name("Done editing").HasClass("PillButton")

		// CUJ: Attempts to change binding to illegal keys.
		s.Log("Editor CUJ #1: key mappings changed to illegal keys")
		if err := uiauto.Combine("mappings changed to illegal keys",
			// Open game dashboard.
			kb.AccelAction("Search+g"),
			// Open game controls editing.
			ui.LeftClickUntil(gameControlsEdit, ui.Gone(gameControlsEdit)),
			// Open the button options menu.
			ui.LeftClickUntil(topTapListItem, ui.Gone(topTapListItem)),
			// Change mapping of space key to "ESC"
			ui.LeftClickUntilFocused(topTapButtonOptionsEditLabel),
			kb.TypeKeyAction(input.KEY_ESC),
			// Verify illegal mapping.
			ui.WaitUntilExists(illegalMappingDescription),
			// Close out.
			ui.LeftClickUntil(buttonOptionsDone, ui.Gone(buttonOptionsDone)),
		)(ctx); err != nil {
			s.Error("Failed to verify illegal keys: ", err)
			// Reset activity.
			if err := gio.CloseAndRelaunchActivity(ctx, &params); err != nil {
				s.Fatal("Failed to reset application after failed CUJ: ", err)
			}
		}

		// CUJ: Key of key binding changed to another existing key bind.
		s.Log("Editor CUJ #2: key mapping changed to a non-existing key bind")
		if err := uiauto.Combine("mapping unbound",
			// Change mapping of "w" to "g"
			ui.LeftClickUntilFocused(topMoveEditingListEditLabel),
			kb.TypeAction("g"),
			// Close editing.
			ui.LeftClickUntil(editingListDone, ui.Gone(editingListDone)),
			// Verify original "w" binding doesn't exist anymore (i.e. the current "w"
			// binding taps at the bottom tap button, not the top tap button).
			gio.MoveOverlayButton(kb, "g", &params),
		)(ctx); err != nil {
			s.Fatal("Failed to verify unbound mapping: ", err)
		}

		// CUJ: Key of key binding changed to another existing key bind.
		s.Log("Editor CUJ #3: key mapping changed to another existing key bind")
		if err := uiauto.Combine("mapping unbound",
			// Open game dashboard.
			kb.AccelAction("Search+g"),
			// Open game controls editing.
			ui.LeftClickUntil(gameControlsEdit, ui.Gone(gameControlsEdit)),
			// Change mapping of "n" to " "
			ui.LeftClickUntilFocused(botTapEditingListEditLabel),
			kb.TypeAction(gio.TopTapKey),
			// Close editing.
			ui.LeftClickUntil(editingListDone, ui.Gone(editingListDone)),
			// Verify original " " binding doesn't exist anymore (i.e. the current " "
			// binding taps at the bottom tap button, not the top tap button).
			gio.TapOverlayButton(kb, gio.TopTapKey, &params, gio.BotTap),
		)(ctx); err != nil {
			s.Fatal("Failed to verify unbound mapping: ", err)
		}

		// CUJ: Close and reopen test application after changing key bindings.
		s.Log("Editor CUJ #4: close and reopen application, after changing key mappings")
		if err := uiauto.Combine("mapping unbound",
			// Close and reopen test application.
			closeAndReopen(&params),
			// Verify " " binding still taps at bottom tap button.
			gio.TapOverlayButton(kb, gio.TopTapKey, &params, gio.BotTap),
		)(ctx); err != nil {
			s.Error("Failed to verify mappings saved after app closure and reopening: ", err)
		}

		return nil
	})
}

// closeAndReopen returns a function that closes the current test application activity and relaunches it.
// It is also important to reassign the "Activity" parameter of the given TestParams
// pointer, since the "SetupTestApp" function called initially defers the closing
// of the original instance of the application.
func closeAndReopen(params *gio.TestParams) action.Action {
	return func(ctx context.Context) error {
		err := gio.CloseAndRelaunchActivity(ctx, params)
		if err != nil {
			return errors.Wrap(err, "failed to create a new ArcInputOverlayTest activity")
		}
		return nil
	}
}
