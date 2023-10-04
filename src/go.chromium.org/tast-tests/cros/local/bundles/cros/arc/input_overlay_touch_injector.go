// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gio"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputOverlayTouchInjector,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test for gaming input overlay touch injector correctness",
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
			}},
		Timeout: 20 * time.Minute,
	})
}

func InputOverlayTouchInjector(ctx context.Context, s *testing.State) {
	gio.SetupTestApp(ctx, s, func(params gio.TestParams) error {
		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)
		// Start up UIAutomator.
		ui := uiauto.New(params.TestConn).WithTimeout(time.Minute)

		if err := uiauto.Combine("Tap overlay keys and ensure proper behavior",
			// Close educational dialog.
			ui.LeftClick(nodewith.Name("Got it").HasClass("LabelButtonLabel")),
			// Execute keystrokes corresponding to tap buttons.
			gio.TapOverlayButton(kb, gio.TopTapKey, &params, gio.TopTap),
			gio.TapOverlayButton(kb, gio.BotTapKey, &params, gio.BotTap),
			// Execute keystrokes corresponding to hold-release controls.
			gio.MoveOverlayButton(kb, gio.UpMoveKey, &params),
			gio.MoveOverlayButton(kb, gio.LeftMoveKey, &params),
			gio.MoveOverlayButton(kb, gio.DownMoveKey, &params),
			gio.MoveOverlayButton(kb, gio.RightMoveKey, &params),
		)(ctx); err != nil {
			return errors.Wrap(err, "one or more keystrokes failed")
		}

		return nil
	})
}
