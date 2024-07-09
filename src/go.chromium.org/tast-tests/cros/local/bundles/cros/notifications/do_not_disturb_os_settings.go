// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package notifications

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

const (
	notificationTitle          = "notificationTitle"
	waitForNotificationTimeout = 30 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DoNotDisturbOSSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks the Do Not Disturb toggle in the OS Settings Notifications subpage",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-consumer-engprod@google.com",
			"newcomer@google.com",
		},
		BugComponent: "b:1246021", // ChromeOS > Software > System UI Surfaces > Notifications
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-08f123fc-aca9-4e05-89ba-b5f4f5fec998",
		}},
		Fixture: "chromeLoggedIn",
	})
}

func DoNotDisturbOSSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn).WithTimeout(waitForNotificationTimeout)

	// Launch Notification Subpage.
	appNotificationPageHeading := nodewith.NameStartingWith("Notifications").Role(role.Heading).Ancestor(ossettings.WindowFinder)
	appSettings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "app-notifications", ui.Exists(appNotificationPageHeading))
	if err != nil {
		s.Fatal("Failed to launch OS Settings")
	}

	// Toggle ON the DND Toggle.
	const dndTitle = "Do Not Disturb"
	if err := appSettings.LeftClick(nodewith.Name(dndTitle).Role(role.ToggleButton))(ctx); err != nil {
		s.Fatal("Failed to toggle on DND: ", err)
	}

	// Confirm that Quick Settings panel also reflects its 'DND' toggle to be on.
	if dndEnabled, err := quicksettings.DoNotDisturbEnabled(ctx, tconn); err != nil {
		s.Fatal("Failed to check if Quick Settings Do Not Disturb toggle is ON: ", err)
	} else if !dndEnabled {
		s.Error("Do Not Disturb toggle is OFF when it should be ON")
	}

	// Confirm that notification doesn't show when DND is toggled on.
	if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic, notificationTitle, "SHOULD NOT SHOW"); err != nil {
		s.Fatal("Failed to create test notification")
	}
	if _, err := ash.WaitForNotification(ctx, tconn, waitForNotificationTimeout, ash.WaitTitle(notificationTitle)); err != nil {
		s.Fatalf("Failed waiting for %v: %v", notificationTitle, err)
	}
	notification := nodewith.Role(role.Window).ClassName("ash/message_center/MessagePopup")
	if err := ui.EnsureGoneFor(notification, waitForNotificationTimeout)(ctx); err != nil {
		s.Fatal("Notification was not suppressed")
	}

	// Toggle OFF the DND Toggle
	if err := appSettings.LeftClick(nodewith.Name(dndTitle).Role(role.ToggleButton))(ctx); err != nil {
		s.Fatal("Failed to toggle off DND: ", err)
	}

	// Confirm that notification shows when DND is toggled off.
	if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic, notificationTitle, "SHOULD SHOW"); err != nil {
		s.Fatal("Failed to create test notification")
	}
	if _, err := ash.WaitForNotification(ctx, tconn, waitForNotificationTimeout, ash.WaitTitle(notificationTitle)); err != nil {
		s.Fatalf("Failed waiting for %v: %v", notificationTitle, err)
	}
	if err := ui.WaitUntilExists(notification)(ctx); err != nil {
		s.Fatal("Failed to find notification popup: ", err)
	}

	// Confirm that Quick Settings panel also reflects its 'DND' toggle to be off.
	if dndEnabled, err := quicksettings.DoNotDisturbEnabled(ctx, tconn); err != nil {
		s.Fatal("Failed to check if Quick Settings Do Not Disturb toggle is OFF: ", err)
	} else if dndEnabled {
		s.Error("Do Not Disturb toggle is ON when it should be OFF")
	}
}
