// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/swap"
	"go.chromium.org/tast-tests/cros/local/arc/vm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SwapNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test vmm-swap and notification interactions",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"kawasin@google.com",
			"hikalium@chromium.org",
		},
		// ChromeOS > Platform > Virtualization > VM Technology
		BugComponent: "b:930563",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"android_vm", "chrome", "crosvm_swap"},
		Fixture:      "arcBootedBoostedVmmSwap",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
	})
}

func SwapNotification(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := a.Install(ctx, arc.APKPath(swap.TestApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	socketPath, err := vm.SocketPath()
	if err != nil {
		s.Fatal("Failed to get crosvm sock: ", err)
	}

	// No Android apps are open on login, so the current fixture's
	// settings should ensure swap is enabled more-or-less immediately.
	s.Log("Waiting for swap to be enabled")
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Pending}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}

	if err := arc.EnsureNotificationPermission(ctx, a, swap.Pkg); err != nil {
		s.Fatal("Failed to ensure the permission: ", err)
	}

	// Create a notification and make sure it shows up.
	if _, err := a.BroadcastIntent(ctx, swap.ReceiverAction, "--include-stopped-packages", swap.Pkg); err != nil {
		s.Fatal("Failed to send broadcast: ", err)
	}

	s.Log("Waiting for notification to be present")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ns, err := ash.Notifications(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get notifications")
		}

		for _, n := range ns {
			if strings.Contains(n.ID, swap.Pkg) {
				return nil
			}
		}
		return errors.New("Notification not found")
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to find notification: ", err)
	}

	// Verify that swap is still enabled when the notification is showing
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Pending}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}

	// Launch the test app from the notification and verify that swap is disabled
	if err := quicksettings.ShowNotificationCenter(ctx, tconn); err != nil {
		s.Fatal("Failed to show quick settings: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	ui := uiauto.New(tconn)
	notificationView := nodewith.HasClass("ArcNotificationContentView").First()
	if err := uiauto.Combine("Click notification",
		ui.WaitUntilExists(notificationView),
		ui.LeftClick(notificationView))(ctx); err != nil {
		s.Fatal("Failed to click on Notifications: ", err)
	}

	s.Log("Waiting for swap to be disabled")
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Ready}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}
}
