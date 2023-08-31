// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package notifications

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NotificationExpandCollapse,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the expand and collapse behavior of a notification",
		Contacts:     []string{"cros-status-area-eng@google.com", "leandre@chromium.org", "chromeos-sw-engprod@google.com"},
		BugComponent: "b:1246021", // ChromeOS > Software > System UI Surfaces > Notifications
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      3 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func NotificationExpandCollapse(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Take a screenshot to show a notification. Using the virtual keyboard is required since
	// different physical keyboards can require different key combinations to take a screenshot.
	vkb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer vkb.Close(ctx)

	ui := uiauto.New(tconn)
	popup := nodewith.Role(role.Window).HasClass("ash/message_center/MessagePopup")
	notificationHeader := nodewith.HasClass("NotificationHeaderView")
	expandButton := nodewith.HasClass("AshNotificationExpandButton")
	statusArea := nodewith.HasClass("ash/StatusAreaWidgetDelegate")
	notificationInMessageCenter := nodewith.HasClass("MessageViewContainer")

	takeScreenshot := func(ctx context.Context) error {
		if err := vkb.Accel(ctx, "Ctrl+F5"); err != nil {
			return errors.Wrap(err, "failed to take a screenshot")
		}

		// Verify that the screen capture popup notification is shown after taking the screenshot.
		if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(popup)(ctx); err != nil {
			return errors.Wrap(err, "failed to find popup notification")
		}
		return nil
	}

	clickNotificationAndVerify := func(ctx context.Context) error {
		// Verify that the Files app is opended after clicking the notification.
		if err := ui.LeftClick(popup)(ctx); err != nil {
			return errors.Wrap(err, "failed to click the notification body")
		}
		if err := ash.WaitForApp(ctx, tconn, apps.FilesSWA.ID, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for Files app to open")
		}

		// Close Files app at the end so we can reopen.
		if err := apps.Close(ctx, tconn, apps.FilesSWA.ID); err != nil {
			return errors.Wrap(err, "failed to close Files app")
		}
		return nil
	}

	clickNotificationInMessageCenterAndVerify := func(ctx context.Context) error {
		// Click the notification in message center and verify opening Files app.
		if err := ui.LeftClick(notificationInMessageCenter)(ctx); err != nil {
			return errors.Wrap(err, "failed to click the notification body")
		}
		if err := ash.WaitForApp(ctx, tconn, apps.FilesSWA.ID, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for Files app to open")
		}

		// Close Files app at the end so we can reopen.
		if err := apps.Close(ctx, tconn, apps.FilesSWA.ID); err != nil {
			return errors.Wrap(err, "failed to close Files app")
		}
		return nil
	}

	if err := uiauto.Combine(
		"generate notification, test collapsing and clicking the notification",
		takeScreenshot,
		ui.WithTimeout(30*time.Second).WaitUntilExists(notificationHeader), // In expanded notification, header is visible.
		ui.LeftClick(expandButton),
		ui.WithTimeout(30*time.Second).WaitUntilGone(notificationHeader), // In collapsed notification, header is not visible.
		clickNotificationAndVerify,
	)(ctx); err != nil {
		s.Fatal("Failed to generate notification, test collapsing and clicking the notification: ", err)
	}

	if err := uiauto.Combine(
		"generate notification, test clicking the expand button twice and verify",
		takeScreenshot,
		ui.LeftClick(expandButton),
		ui.LeftClick(expandButton),
		ui.WithTimeout(30*time.Second).WaitUntilExists(notificationHeader), // In expanded notification, header is visible.
		clickNotificationAndVerify,
	)(ctx); err != nil {
		s.Fatal("Failed to generate notification, test clicking the expand button twice and verify: ", err)
	}

	if err := uiauto.Combine(
		"generate notification, test expand/collapse in message center",
		takeScreenshot,
		ui.LeftClick(statusArea),
		ui.WithTimeout(30*time.Second).WaitUntilExists(notificationHeader), // In expanded notification, header is visible.
		// Collapse the notification. Then close and reopen message center.
		// Notification should remain collapsed (keeping the old state before closing and reopening).
		ui.LeftClick(expandButton),
		ui.LeftClick(statusArea),
		ui.LeftClick(statusArea),
		ui.WithTimeout(30*time.Second).WaitUntilGone(notificationHeader), // In collapsed notification, header is not visible.
		clickNotificationInMessageCenterAndVerify,
	)(ctx); err != nil {
		s.Fatal("Failed to generate notification, test expand/collapse in message center: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	// Remove all screenshots at the end.
	if err = screenshot.RemoveScreenshots(downloadsPath); err != nil {
		s.Fatal("Failed to remove screenshots: ", err)
	}
}
