// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package notifications

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NotificationImageDrag,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the notification image drag feature works as expected",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-consumer-engprod@google.com",
			"andrewxu@chromium.org",
		},
		BugComponent: "b:1246021", // Chrome OS > Software > System UI Surfaces > Notifications
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func NotificationImageDrag(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(ctx)

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Open a blank web page.
	conn, err := cr.NewConn(ctx, "about:blank")
	if err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer conn.Close()

	vkb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer vkb.Close(ctx)

	layout, err := input.KeyboardTopRowLayout(ctx, vkb)
	if err != nil {
		s.Fatal("Failed to get the keyboard top row layout: ", err)
	}

	ui := uiauto.New(tconn)
	notificationImage := nodewith.ClassName("LargeImageView")

	// Generate a screen capture notification then drag the notification image
	// to the browser window.
	if err := uiauto.Combine("drag the notification image to the browser",
		vkb.AccelAction("Ctrl+"+layout.SelectTask),
		ui.WithTimeout(30*time.Second).WaitUntilExists(notificationImage),
		ui.MousePress(mouse.LeftButton, notificationImage),
		ui.MouseMoveTo(nodewith.ClassName("ContentsWebView").Role(role.WebView), 500*time.Millisecond),
		ui.MouseRelease(mouse.LeftButton),
		ui.WaitUntilGone(nodewith.Role(role.Window).ClassName("ash/message_center/MessagePopup")),
		ui.WaitUntilGone(nodewith.ClassName("NotificationCounterView")))(ctx); err != nil {
		s.Fatal("Failed to drag the notification image to the web browser: ", err)
	}
}
