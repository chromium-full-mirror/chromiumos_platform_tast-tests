// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FullRestoreFilesappReboot,
		Desc: "Test full restore files app",
		Contacts: []string{
			"cros-web-apps-team@google.com",
			"renkens@google.com",
		},
		BugComponent: "b:1389907",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

func FullRestoreFilesappReboot(ctx context.Context, s *testing.State) {
	func() {
		cr, err := chrome.New(ctx)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}

		// Open the Files app.
		// The opened Files app is not closed before reboot so that it could be restored after reboot.
		_, err = filesapp.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch Files app: ", err)
		}

		// GoBigSleepLint: According to the PRD of Full Restore go/chrome-os-full-restore-dd,
		// it uses a throttle of 2.5s to save the app launching and window statue information to the backend.
		// Therefore, sleep 5 seconds here.
		// TODO(b/394258768): Find a way to wait for the sync instead of sleeping.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}()

	func() {
		cr, err := chrome.New(ctx,
			// Set not to clear the notification after restore.
			// By default, On startup is set to ask every time after reboot
			// and there is a window asking the user to select whether to restore or not.
			chrome.RemoveNotification(false),
			chrome.KeepState())
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(ctx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}

		restoreWidget := nodewith.ClassName("InformedRestoreWidget").Role(role.Window)
		openButton := nodewith.Name("Open").Role(role.Button).Ancestor(restoreWidget)
		downloads := nodewith.Name(filesapp.Downloads).Role(role.TreeItem).Ancestor(filesapp.WindowFinder(apps.FilesSWA.ID))

		ui := uiauto.New(tconn)
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

		if err := uiauto.Combine("restore files app",
			// Click Open on the Welcome Recap window.
			ui.WithTimeout(30*time.Second).LeftClick(openButton),

			// Check Files app is restored.
			ui.WaitUntilExists(downloads))(ctx); err != nil {
			s.Fatal("Failed to restore Files app: ", err)
		}
	}()
}
