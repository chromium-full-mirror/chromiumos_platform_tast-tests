// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShowLowDiskSpaceNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test showing the low disk space notification",
		BugComponent: "b:1264602", // ChromeOS > Software > Commercial (Enterprise) > DiskCleanup
		Contacts: []string{
			"chromeos-disk-cleanup@google.com",
			"vsavu@google.com",     // Test author
			"gwendal@chromium.org", // Lead for ChromeOS Storage
		},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:cryptohome",
		},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
		HardwareDeps: hwdep.D(hwdep.MinStorage(16)),
	})
}

func ShowLowDiskSpaceNotification(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	fillFile, err := disk.FillUntil(cleanup.UserHome, cleanup.MinimalFreeSpace)
	if err != nil {
		s.Fatal("Failed to fill disk space: ", err)
	}
	defer func() {
		if err := os.Remove(fillFile); err != nil {
			s.Errorf("Failed to remove fill file %s: %v", fillFile, err)
		}
	}()

	const notificationWaitTime = 150 * time.Second // Timeout for checking for low disk space notification.
	const notificationID = "low_disk"              // Hardcoded in Chrome.

	s.Logf("Waiting %d seconds for %v notification", notificationWaitTime/time.Second, notificationID)
	if _, err := ash.WaitForNotification(ctx, tconn, notificationWaitTime, ash.WaitIDContains(notificationID)); err != nil {
		// Check if too much space was made available.
		freeSpace, fErr := disk.FreeSpace(cleanup.UserHome)
		if fErr != nil {
			s.Fatal("Failed to read the amount of free space", fErr)
		}

		if freeSpace >= cleanup.NotificationThreshold {
			s.Errorf("Space was cleaned without notification: got %d; want < %d", freeSpace, cleanup.NotificationThreshold)
		}

		s.Error("Notification not shown: ", err)
	}
}
