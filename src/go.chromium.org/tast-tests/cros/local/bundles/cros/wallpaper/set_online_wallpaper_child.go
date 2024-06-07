// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast/core/testing"
)

const notificationID = "time-limit-screen-time"

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetOnlineWallpaperChild,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting online wallpapers in the new wallpaper app for a child user",
		Contacts: []string{
			"cros-personalization@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-2502e146-d4a3-4251-baba-d3cd24f18b45",
		}},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"unicorn.wallpaperCategory", "unicorn.wallpaperName"},
		Fixture:      "familyLinkUnicornLogin",
		Timeout:      5 * time.Minute,
	})
}

func SetOnlineWallpaperChild(ctx context.Context, s *testing.State) {
	collection := s.RequiredVar("unicorn.wallpaperCategory")
	image := s.RequiredVar("unicorn.wallpaperName")

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	// Force Chrome to be in clamshell mode to make sure wallpaper preview is not
	// enabled.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(ctx)

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// If family link notification appear after logging in, it will interfere with the test if we start at the same time.
	// Close the notification if it appears, then check if the notification is already gone before starting the test.
	if _, err := ash.WaitForNotification(ctx, tconn, 20*time.Second, ash.WaitIDContains(notificationID)); err != nil {
		s.Log("Family link notification not showing up: ", err)
	}

	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}

	if err := ash.WaitUntilNotificationGone(ctx, tconn, 10*time.Second, ash.WaitIDContains(notificationID)); err != nil {
		s.Fatal("Failed to wait for family link notification to disappear: ", err)
	}

	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", collection, image),
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, collection),
		wallpaper.SelectImage(ui, image),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(image)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", collection, image, err)
	}
}
