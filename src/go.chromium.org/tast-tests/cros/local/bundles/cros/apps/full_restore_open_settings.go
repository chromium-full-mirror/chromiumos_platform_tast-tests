// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FullRestoreOpenSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test full restore notification and browser",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"nancylingwang@google.com",
		},
		BugComponent: "crbug:Platform>Apps>Foundation",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Vars:         []string{"ui.gaiaPoolDefault"},
		SoftwareDeps: []string{"chrome"},
	})
}

func FullRestoreOpenSettings(ctx context.Context, s *testing.State) {
	func() {
		cr, err := chrome.New(ctx, chrome.EnableFeatures("FullRestore"))
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}

		// Open browser.
		// The opened browser is not closed before reboot so that it could be restored after reboot.
		_, err = browser.Launch(ctx, tconn, cr, "https://abc.xyz")
		if err != nil {
			s.Fatal("Failed to launch browser: ", err)
		}

		// According to the PRD of Full Restore go/chrome-os-full-restore-dd,
		// it uses a throttle of 2.5s to save the app launching and window statue information to the backend.
		// Therefore, sleep 5 seconds here.
		testing.Sleep(ctx, 5*time.Second)

	}()

	func() {
		cr, err := chrome.New(ctx,
			// Set not to clear the notification after restore.
			// By default, On startup is set to ask every time after reboot
			// and there is an alertdialog asking the user to select whether to restore or not.
			chrome.RemoveNotification(false),
			chrome.EnableFeatures("FullRestore"),
			chrome.EnableRestoreTabs(),
			chrome.KeepState())
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}

		alertDialog := nodewith.NameStartingWith("Restore apps?").Role(role.AlertDialog)
		settingsButton := nodewith.Name("Settings").Role(role.Button).Ancestor(alertDialog)

		ui := uiauto.New(tconn)
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

		if err := ui.LeftClick(settingsButton)(ctx); err != nil {
			s.Fatal("Failed to click notification SETTINGS button: ", err)
		}

		if err := ui.WaitUntilGone(alertDialog)(ctx); err != nil {
			s.Fatal("Failed to wait for notification to disappear: ", err)
		}

		if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
			s.Fatal("Settings app did not appear in the shelf: ", err)
		}

		// Confirm that the Settings apps page is open.
		appsPage := nodewith.Name("Apps").Role(role.Heading).Ancestor(ossettings.WindowFinder)
		if err := uiauto.New(tconn).WaitUntilExists(appsPage)(ctx); err != nil {
			s.Fatal("Failed to wait for Settings apps page: ", err)
		}

		// After clicking the SETTINGS button, the notification is hidden in the system tray.
		// Show the notification center to find the notification.
		if err := quicksettings.ShowNotificationCenter(ctx, tconn); err != nil {
			s.Fatal("Failed to show notification center: ", err)
		}

		notificationDialog := nodewith.HasClass("AshNotificationView").NameStartingWith("Restore apps?")
		expandButton := nodewith.Name("Expand notification").Role(role.Button).Ancestor(notificationDialog)
		// The restore notification is collapsed sometimes, expand it.
		if err := uiauto.IfSuccessThen(ui.WaitUntilExists(expandButton), ui.LeftClick(expandButton))(ctx); err != nil {
			s.Fatal("Failed to expand Restore notification: ", err)
		}

		restoreButton := nodewith.Name("Restore").Role(role.Button).Ancestor(notificationDialog)
		if err := ui.LeftClick(restoreButton)(ctx); err != nil {
			s.Fatal("Failed to click notification RESTORE button: ", err)
		}

		if err := ui.WaitUntilGone(restoreButton)(ctx); err != nil {
			s.Fatal("Failed to wait for notification to close: ", err)
		}

		if _, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool { return w.WindowType == ash.WindowTypeBrowser }); err != nil {
			s.Fatal("Failed to restore browser: ", err)
		}
	}()
}
