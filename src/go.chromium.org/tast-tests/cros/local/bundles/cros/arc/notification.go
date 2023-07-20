// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Notification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launches a testing APK to generate notification and verifies its state",
		Contacts: []string{
			"arc-framework+tast@google.com",
			"yhanada@chromium.org",
			"toshikikikuchi@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Framework > Notifications
		BugComponent: "b:537324",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		Fixture:      "arcBooted",
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:arc-functional"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:arc-functional"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "refresh",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "refresh_vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func Notification(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	const (
		apk = "ArcNotificationTest.apk"
		pkg = "org.chromium.arc.testapp.notification"
		cls = ".NotificationActivity"

		// UI IDs in the app.
		idPrefix = pkg + ":id/"
		titleID  = idPrefix + "notification_title"
		textID   = idPrefix + "notification_text"
		idID     = idPrefix + "notification_id"
		sendID   = idPrefix + "send_button"
		removeID = idPrefix + "remove_button"

		// Button id of GrantPermissionsActivity's "ALLOW" button
		permissionAllowBtnID = "com.android.permissioncontroller:id/permission_allow_button"

		// Testing data.
		title  = "title!"
		title2 = "new title!"
		text   = "hi from Tast"
		msgID  = "12345"

		// Notification ID on Android is composed of many components.
		// This is the substring to match the notification generated
		// earlier.
		notificationID = "|" + pkg + "|" + msgID + "|"
	)

	s.Logf("Installing %s", apk)
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatalf("Failed to install %s: %v", apk, err)
	}

	s.Logf("Ensuring the permission for %s", pkg)
	if err := arc.EnsureNotificationPermission(ctx, a, pkg); err != nil {
		s.Fatal("Failed to ensure the permission: ", err)
	}

	s.Log("Launching app")
	act, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		s.Fatal("Failed to create a new activity: ", err)
	}

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start the activity: ", err)
	}

	s.Log("Setup is done, and running the test scenario")

	// Create a notification.
	if err := d.Object(ui.ID(titleID)).SetText(ctx, title); err != nil {
		s.Fatalf("Failed to set text to %s: %v", titleID, err)
	}
	if err := d.Object(ui.ID(textID)).SetText(ctx, text); err != nil {
		s.Fatalf("Failed to set text to %s: %v", textID, err)
	}
	if err := d.Object(ui.ID(idID)).SetText(ctx, msgID); err != nil {
		s.Fatalf("Failed to set text to %s: %v", idID, err)
	}
	if err := d.Object(ui.ID(sendID)).Click(ctx); err != nil {
		s.Fatalf("Failed to click %s button: %v", sendID, err)
	}

	//  Default message timeout, Messages should be almost instant, and 10 seconds is very generous margin of error.
	timeout := 10 * time.Second

	//  Wait for the initial notification to show.
	_, err = ash.WaitForNotification(ctx, tconn, timeout, ash.WaitIDContains(notificationID), ash.WaitTitle(title), ash.WaitMessageContains(text))
	if err != nil {
		s.Fatalf("Expected notification for %s did not show", title)
	}

	// Update the title.
	if err := d.Object(ui.ID(titleID)).SetText(ctx, title2); err != nil {
		s.Fatalf("Failed to set text to %s: %v", titleID, err)
	}
	if err := d.Object(ui.ID(sendID)).Click(ctx); err != nil {
		s.Fatalf("Failed to click %s button: %v", sendID, err)
	}

	// Wait for that the title is updated in new notification.
	_, err = ash.WaitForNotification(ctx, tconn, timeout, ash.WaitIDContains(notificationID), ash.WaitTitle(title2), ash.WaitMessageContains(text))
	if err != nil {
		s.Fatalf("Expected notification for %s did not show: %v", title2, err)
	}

	// Remove the notification.
	if err = ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}
	// Ensure notification was removed.
	if err = ash.WaitUntilNotificationGone(ctx, tconn, timeout, ash.WaitIDContains(notificationID), ash.WaitTitle(title2), ash.WaitMessageContains(text)); err != nil {
		s.Fatal("Notification wasn't removed: ", err)
	}
}
