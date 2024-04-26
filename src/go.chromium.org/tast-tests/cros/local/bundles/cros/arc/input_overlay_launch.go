// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputOverlayLaunch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Installs the GIO test application and checks for launch correctness",
		Contacts:     []string{"arc-app-dev@google.com", "pjlee@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
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

func InputOverlayLaunch(ctx context.Context, s *testing.State) {
	gio.SetupTestApp(ctx, s, func(params gio.TestParams) error {
		// Start up UIAutomator.
		ui := uiauto.New(params.TestConn).WithTimeout(time.Minute)

		if err := uiauto.Combine("Find gaming input overlay UI elements",
			// Find input overlay tap buttons.
			ui.WaitUntilExists(nodewith.Name(gio.TopTapKeyName).HasClass("LabelButtonLabel")),
			ui.WaitUntilExists(nodewith.Name(gio.BotTapKey).HasClass("LabelButtonLabel")),
			// Find input overlay joystick buttons.
			ui.WaitUntilExists(nodewith.Name(gio.UpMoveKey).HasClass("LabelButtonLabel")),
			ui.WaitUntilExists(nodewith.Name(gio.RightMoveKey).HasClass("LabelButtonLabel")),
			ui.WaitUntilExists(nodewith.Name(gio.DownMoveKey).HasClass("LabelButtonLabel")),
			ui.WaitUntilExists(nodewith.Name(gio.LeftMoveKey).HasClass("LabelButtonLabel")),
		)(ctx); err != nil {
			return errors.Wrap(err, "one or more items not loaded")
		}

		return nil
	})
}
