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
		Func:     InputOverlayDisplay,
		Desc:     "Test for gaming input overlay menu correctness",
		Contacts: []string{"arc-app-dev@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithGameDashboard",
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
	})
}

func InputOverlayDisplay(ctx context.Context, s *testing.State) {
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

		// CUJ: Hide game overlay.
		s.Log("Display CUJ #1: hide game overlay")
		topTapKey := nodewith.Name(gio.TopTapKeyName).HasClass("LabelButtonLabel")
		upMoveKey := nodewith.Name(gio.UpMoveKey).HasClass("LabelButtonLabel")
		gameDashboardButton := nodewith.Name("Game dashboard").HasClass("GameDashboardButton")
		gameControlsHintButton := nodewith.NameContaining("controls").HasClass("FeatureTile")
		gameControlsFeatureButton := nodewith.HasClass("Switch")
		gameControlsEdit := nodewith.NameContaining("controls").HasClass("GameDashboardMainMenuView::GameControlsDetailsRow")
		editingListDone := nodewith.Name("Done editing").HasClass("PillButton")

		if err := uiauto.Combine("hide game overlay",
			// Open game dashboard main menu.
			ui.LeftClickUntil(gameDashboardButton, ui.Exists(gameControlsHintButton)),
			// Hide Game Controls.
			ui.LeftClick(gameControlsHintButton),
			// Close game dashboard main menu.
			ui.LeftClick(gameDashboardButton),
			// Poll UI elements no longer exist, but overlay is still responsive.
			ui.Gone(topTapKey),
			gio.TapOverlayButton(kb, gio.TopTapKey, &params, gio.TopTap),
			ui.Gone(upMoveKey),
			gio.MoveOverlayButton(kb, gio.UpMoveKey, &params),
			// Poll edits can still be done.
			ui.LeftClickUntil(gameDashboardButton, ui.Exists(gameControlsHintButton)),
			ui.LeftClick(gameControlsEdit),
			ui.WaitUntilExists(topTapKey.First()),
			ui.WaitUntilExists(upMoveKey.First()),
			// Exit out.
			ui.LeftClick(editingListDone),
		)(ctx); err != nil {
			s.Error("Failed to verify game overlay hidden: ", err)
			// Reset activity.
			if err := gio.CloseAndRelaunchActivity(ctx, &params); err != nil {
				s.Fatal("Failed to reset application after failed CUJ: ", err)
			}
		}

		// CUJ: Disable game overlay.
		s.Log("Display CUJ #2: disable game overlay")
		if err := uiauto.Combine("disable game overlay",
			// Open game dashboard main menu.
			ui.LeftClickUntil(gameDashboardButton, ui.Exists(gameControlsFeatureButton)),
			// Tap Controls switch button.
			ui.LeftClick(gameControlsFeatureButton),
			ui.LeftClick(gameDashboardButton),
			// Poll UI elements no longer exist, and overlay is unresponsive.
			ui.Gone(topTapKey),
			not(gio.TapOverlayButton(kb, gio.TopTapKey, &params, gio.TopTap)),
			ui.Gone(upMoveKey),
			not(gio.MoveOverlayButton(kb, gio.UpMoveKey, &params)),
			ui.LeftClick(gameDashboardButton),
			// Check edit button disabled.
			ui.LeftClick(gameControlsEdit),
			not(ui.Gone(gameControlsEdit)),
		)(ctx); err != nil {
			s.Error("Failed to verify game overlay disabled: ", err)
		}

		return nil
	})
}

// not returns a function that returns an error if the given action did not return
// an error, and returns nil if the given action resulted in an error.
func not(a action.Action) action.Action {
	return func(ctx context.Context) error {
		if err := a(ctx); err == nil {
			return errors.Wrap(err, "action succeeded unexpectedly")
		}
		return nil
	}
}
