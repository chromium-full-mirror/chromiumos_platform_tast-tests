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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputOverlayEditor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test for gaming input overlay key mapping editor correctness",
		Contacts:     []string{"arc-app-dev@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Framework > Gaming
		BugComponent: "b:767470",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithInputOverlayAlphaV2",
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
		// Start up ACUITI.
		uda := uidetection.NewDefault(params.TestConn).WithOptions(uidetection.Retries(3)).WithTimeout(time.Minute)

		editButton := nodewith.Name("Edit").HasClass("PillButton")
		appWindow := nodewith.Name("ARC Input Overlay Test").Role(role.Window).HasClass("RootView")

		// CUJ: Attempts to change binding to illegal keys.
		s.Log("Editor CUJ #1: key mappings changed to illegal keys")
		if err := uiauto.Combine("mappings changed to illegal keys",
			// Close educational dialog.
			ui.LeftClick(nodewith.Name("Got it").HasClass("LabelButtonLabel")),
			// Open game controls.
			ui.LeftClick(nodewith.Name("Game controls").HasClass("MenuEntryView")),
			ui.LeftClick(editButton),
			// Change mapping of "w" to "ESC" (NOTE: "w" key is used because, unlike the
			// "n" key, the associated on-screen error messages have no overlapping text,
			// and thus it has the highest chance of success with text detection).
			ui.LeftClick(nodewith.Name(gio.UpMoveKey).HasClass("LabelButtonLabel")),
			kb.TypeKeyAction(input.KEY_ESC),
			// Verify illegal mapping.
			waitForMultiple(uda, "following", "supported", "Volume"),
			// Change mapping of "w" to "w".
			kb.TypeAction(gio.UpMoveKey),
			// Verify illegal mapping.
			waitForMultiple(uda, "Same", "ame"),
			// Change mapping of "w" to "CTRL"
			kb.TypeKeyAction(input.KEY_LEFTCTRL),
			// Verify illegal mapping.
			waitForMultiple(uda, "following", "supported", "Volume"),
			// Close out.
			uda.Tap(uidetection.Word("Cancel").WithinA11yNode(appWindow)),
		)(ctx); err != nil {
			s.Error("Failed to verify illegal keys: ", err)
			// Reset activity.
			if err := gio.CloseAndRelaunchActivity(ctx, &params); err != nil {
				s.Fatal("Failed to reset application after failed CUJ: ", err)
			}
		}

		// CUJ: Change key mappings and then press cancel.
		s.Log("Editor CUJ #2: key mappings changes canceled")
		if err := uiauto.Combine("cancel changed mapping",
			// Open game controls.
			ui.LeftClick(nodewith.Name("Game controls").HasClass("MenuEntryView")),
			ui.LeftClick(editButton),
			// Change mapping of "n" to "l".
			ui.LeftClick(nodewith.Name(gio.BotTapKey).HasClass("LabelButtonLabel")),
			kb.TypeAction("l"),
			uda.Tap(uidetection.Word("Cancel").WithinA11yNode(appWindow)),
			// Verify old mapping still exists.
			ui.WaitUntilExists(nodewith.Name(gio.BotTapKey).HasClass("LabelButtonLabel")),
		)(ctx); err != nil {
			s.Error("Failed to verify canceled mapping: ", err)
			// Reset activity.
			if err := gio.CloseAndRelaunchActivity(ctx, &params); err != nil {
				s.Fatal("Failed to reset application after failed CUJ: ", err)
			}
		}

		// CUJ: Key of key binding changed to another existing key bind.
		s.Log("Editor CUJ #3: key mapping changed to a non-existing key bind")
		if err := uiauto.Combine("mapping unbound",
			// Open game controls.
			ui.LeftClick(nodewith.Name("Game controls").HasClass("MenuEntryView")),
			ui.LeftClick(editButton),
			// Change mapping of "w" to "g"
			ui.LeftClick(nodewith.Name(gio.UpMoveKey).HasClass("LabelButtonLabel")),
			kb.TypeAction("g"),
			// Save binding.
			uda.Tap(uidetection.Word("Save")),
			uda.WaitUntilGone(uidetection.Word("Save")),
			// Verify original "w" binding doesn't exist anymore (i.e. the current "w"
			// binding taps at the bottom tap button, not the top tap button).
			gio.MoveOverlayButton(kb, "g", &params),
		)(ctx); err != nil {
			s.Fatal("Failed to verify unbound mapping: ", err)
		}

		// CUJ: Key of key binding changed to another existing key bind.
		s.Log("Editor CUJ #4: key mapping changed to another existing key bind")
		if err := uiauto.Combine("mapping unbound",
			// Open game controls.
			ui.LeftClick(nodewith.Name("Game controls").HasClass("MenuEntryView")),
			ui.LeftClick(editButton),
			// Change mapping of "n" to " "
			ui.LeftClick(nodewith.Name(gio.BotTapKey).HasClass("LabelButtonLabel")),
			kb.TypeAction(gio.TopTapKey),
			// Save binding.
			uda.Tap(uidetection.Word("Save")),
			uda.WaitUntilGone(uidetection.Word("Save")),
			// Verify original " " binding doesn't exist anymore (i.e. the current " "
			// binding taps at the bottom tap button, not the top tap button).
			gio.TapOverlayButton(kb, gio.TopTapKey, &params, gio.BotTap),
		)(ctx); err != nil {
			s.Fatal("Failed to verify unbound mapping: ", err)
		}

		// CUJ: Close and reopen test application after changing key bindings.
		s.Log("Editor CUJ #5: close and reopen application, after changing key mappings")
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

// waitForMultiple returns true if any of the listed words are found via ACUITI.
func waitForMultiple(uda *uidetection.Context, words ...string) action.Action {
	return func(ctx context.Context) error {
		for _, word := range words {
			if err := uda.WaitUntilExists(uidetection.Word(word))(ctx); err != nil {
				continue
			}
			return nil
		}
		return errors.New("no listed words found")
	}
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
