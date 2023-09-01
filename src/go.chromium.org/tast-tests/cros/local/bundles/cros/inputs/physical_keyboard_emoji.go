// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/ime/emojipicker"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardEmoji,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that right click input field and select emoji with physical keyboard",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"inputs_deps", "chrome", "chrome_internal"},
		SearchFlags: util.SearchFlagsWithIMEAndScreenPlay(
			[]ime.InputMethod{ime.DefaultInputMethod},
			[]string{
				"screenplay-635feda2-f278-4f00-94a7-b25c849a57fd",
				"screenplay-1187c2a2-f40a-4f0c-9cfa-b235085fb37f",
				"screenplay-f58d2b39-4a8d-45ec-88a3-3c7a6b015bbb",
				"screenplay-0b77f4bd-6b19-4723-8428-62c414453364",
			}),
		Params: []testing.Param{
			{
				Fixture:           fixture.ClamshellNonVK,
				ExtraAttr:         []string{"informational"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...), hwdep.SkipOnModel("kefka")),
			},
			{
				Name:      "informational",
				Fixture:   fixture.ClamshellNonVK,
				ExtraAttr: []string{"informational"},
				// Skip on grunt & zork boards due to b/213400835.
				ExtraHardwareDeps: hwdep.D(pre.InputsUnstableModels, hwdep.SkipOnPlatform("grunt", "zork")),
			},
			{
				Name:              "lacros",
				Fixture:           fixture.LacrosClamshellNonVK,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:input-tools-upstream"},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...), hwdep.SkipOnModel("kefka")),
			},
		},
	})
}

func PhysicalKeyboardEmoji(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	inputField := testserver.TextAreaInputField
	inputEmoji := "😂"
	ui := emojipicker.NewUICtx(tconn)

	s.Run(ctx, "emoji_input", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_emoji_input")
		if err := its.InputEmojiWithEmojiPicker(uc, inputField, inputEmoji)(ctx); err != nil {
			s.Fatal("Failed to verify emoji picker: ", err)
		}
	})

	// Tap ESC key to dismiss emoji picker.
	// This test is also covered in browser test https://source.chromium.org/chromium/chromium/src/+/main:chrome/browser/ui/views/bubble/bubble_contents_wrapper_unittest.cc;drc=7059ce9510b276afe73ce0bc389a72b58f482420;l=154.
	// Keep this test since it is still required to complete the entire E2E test journey.
	dismissByKeyboardAction := uiauto.UserAction(
		"Dismiss emoji picker",
		uiauto.Combine("dismiss emoji picker by tapping ESC key",
			kb.AccelAction("ESC"),
			emojipicker.WaitUntilGone(tconn),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeTestScenario: "Dismiss emoji picker by tapping ESC key",
				useractions.AttributeFeature:      useractions.FeatureEmojiPicker,
			},
		},
	)

	// Mouse click to dismiss emoji picker.
	dismissByMouseAction := uiauto.UserAction(
		"Dismiss emoji picker",
		uiauto.Combine("dismiss emoji picker by mouse click",
			func(ctx context.Context) error {
				emojiPickerLoc, err := ui.Location(ctx, emojipicker.RootFinder)
				if err != nil {
					return errors.Wrap(err, "failed to get emoji picker location")
				}
				// Click anywhere outside emoji picker will dismiss it.
				// Using TopRight + 50 is safe in this case.
				clickPoint := coords.Point{
					X: emojiPickerLoc.TopRight().X + 50,
					Y: emojiPickerLoc.TopRight().Y,
				}
				return ui.MouseClickAtLocation(0, clickPoint)(ctx)
			},
			emojipicker.WaitUntilGone(tconn),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeTestScenario: "Dismiss emoji picker by mouse click",
				useractions.AttributeFeature:      useractions.FeatureEmojiPicker,
			},
		},
	)

	s.Run(ctx, "recently_used", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_recently")

		if err := uiauto.Combine("validate recently used emojis",
			its.TriggerEmojiPickerFromContextMenu(inputField),
			its.DismissGifNudgeOverlay(),
			// Clear recent used emojis.
			uiauto.UserAction(
				"Clear recently used emoji",
				uiauto.Combine("clear recently used emoji",
					ui.LeftClick(emojipicker.RecentUsedMenu),
					ui.LeftClick(emojipicker.ClearRecentlyUsedButtonFinder),
					ui.WaitUntilGone(emojipicker.RecentUsedMenu),
					dismissByKeyboardAction,
					// Launch emoji picker again to confirm it is not only removed from UI.
					its.TriggerEmojiPickerFromContextMenu(inputField),
					ui.WaitUntilGone(emojipicker.RecentUsedMenu),
					dismissByMouseAction,
				),
				uc,
				&useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeFeature: useractions.FeatureEmojiPicker,
					},
				},
			),
		)(ctx); err != nil {
			s.Fatal("Failed to clear recently used emoji: ", err)
		}
	})
}
