// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package notifications

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const uiTimeout = 30 * time.Second

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that notifications appear in notification centre and can be interacted with",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"amehfooz@chromium.org",
			"cros-system-ui-eng@google.com",
		},
		BugComponent: "b:1246021", // ChromeOS > Software > System UI Surfaces > Notifications
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-b07d14ae-c58d-4b7d-9108-faeaee45af4c",
		}, {
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// Smoke tests that notifications appear in notification centre.
func Smoke(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Setup a browser.
	bt := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(ctx)
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatalf("Failed to create Test API connection for %v browser: %v", bt, err)
	}

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(ctx)

	s.Log("Creating notification")
	if _, err := browser.CreateTestNotification(ctx, bTconn, browser.NotificationTypeBasic, "TestNotification1", "blahhh"); err != nil {
		s.Fatal("Failed to create test notification: ", err)
	}
	s.Log("Checking that notification appears")
	ui := uiauto.New(tconn)

	// Ensure mouse is not hovered over the area the notification shows up in, this
	// would result in the notification dismiss timer staying paused
	s.Log("Moving mouse pointer to home button")
	if err := ui.MouseMoveTo(nodewith.ClassName("ash/HomeButton"), 0)(ctx); err != nil {
		s.Fatal("Failed to move mouse: ", err)
	}

	notification := nodewith.Role(role.Window).ClassName("ash/message_center/MessagePopup")
	if err := ui.WithTimeout(uiTimeout).WaitUntilExists(notification)(ctx); err != nil {
		s.Fatal("Failed to find notification popup: ", err)
	}
	s.Log("Waiting for notification to auto-dismiss")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.Exists(notification)(ctx); err != nil {
			// Notification dismissed.
			return nil
		}
		// Notification still exists.
		// Depending on DUT state when test is run, the notification could be focused and will not autodismiss.
		// Check for that condition while waiting for notification to dismiss.
		if err := ui.Exists(nodewith.State(state.Focused, true).Ancestor(notification))(ctx); err != nil {
			return errors.New("focused notification does not exist")
		}
		s.Log("Notification had focus. Opening launcher to change focus")
		if err := launcher.Open(tconn)(ctx); err != nil {
			s.Fatal("Failed to open launcher: ", err)
		}
		return errors.New("notification exists and was focused")
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed waiting for notification to dismiss: ", err)
	}
	s.Log("Open notification center")
	if err := quicksettings.ShowNotificationCenter(ctx, tconn); err != nil {
		s.Fatal("Failed to show notification center: ", err)
	}
	// Closing all notifications ensures the notification center closes.
	defer ash.CloseNotifications(ctx, tconn)

	s.Log("Closing notification from notification centre")
	if err := closeNotification(ctx, tconn); err != nil {
		s.Fatal("Failed to close notification: ", err)
	}
	s.Log("Firing another notification while notification centre is open")
	if _, err := browser.CreateTestNotification(ctx, bTconn, browser.NotificationTypeBasic, "TestNotification2", "testttt"); err != nil {
		s.Fatal("Failed to create test notification: ", err)
	}
	s.Log("Closing notification from notification centre")
	if err := closeNotification(ctx, tconn); err != nil {
		s.Fatal("Failed to close notification: ", err)
	}
}

func closeNotification(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(uiTimeout)
	notificationClose := nodewith.Role(role.Button).Name("Notification close")
	if err := ui.LeftClick(notificationClose)(ctx); err != nil {
		return errors.Wrap(err, "failed to click notification close button")
	}
	if err := ui.WaitUntilGone(notificationClose)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for closed notification to disappear")
	}
	return nil
}
