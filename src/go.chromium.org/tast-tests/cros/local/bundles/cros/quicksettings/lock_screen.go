// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LockScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the screen can be locked from Quick Settings",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"amehfooz@chromium.org",
			"cros-system-ui-eng@google.com",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
			Val:               true,
		}, {
			Name: "no_battery",
			Val:  false,
		}},
	})
}

// LockScreen tests that the screen can be locked from Quick Settings
// and verifies its contents when the screen is locked.
func LockScreen(ctx context.Context, s *testing.State) {
	const (
		username = "testuser@gmail.com"
		password = "pass"

		lockTimeout = 30 * time.Second
	)

	cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: username, Pass: password}),
		chrome.EnableFeatures("QsRevamp"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// Show a test notification.
	title := "Test Title"
	if _, err := browser.CreateTestNotification(ctx, tconn, browser.NotificationTypeBasic, title, "message"); err != nil {
		s.Fatal("Failed to create test notification")
	}
	if _, err := ash.WaitForNotification(ctx, tconn, 5*time.Second, ash.WaitTitle(title)); err != nil {
		s.Fatal("Failed waiting for notification")
	}

	if err := quicksettings.LockScreen(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}
	// Unlock the screen to ensure subsequent tests aren't affected by the screen remaining locked.
	// TODO(crbug/1156812): Remove once chrome.go has a way to clean up the lock screen state.
	defer func() {
		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, lockTimeout); err != nil {
			s.Fatalf("Waiting for screen to be ready for password failed: %v (last status %+v)", err, st)
		}

		if err := lockscreen.EnterPassword(ctx, tconn, username, password, keyboard); err != nil {
			s.Fatal("Entering password failed: ", err)
		}

		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.LoggedIn }, 30*time.Second); err != nil {
			s.Fatalf("Failed waiting to log in: %v, last state: %+v", err, st)
		}
	}()

	// Check if notifications are hidden. This opens the notifications center.
	if hidden, err := quicksettings.NotificationsHidden(ctx, tconn); err != nil {
		s.Fatal("Failed to check if notifications were hidden: ", err)
	} else if !hidden {
		s.Fatal("Notifications were not hidden")
	}

	// Explicitly show Quick Settings on the lock screen, so it will
	// remain open for the UI verification steps.
	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show quick settings on the lock screen: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	// Get the restricted feature tiles.
	restrictedTiles, err := quicksettings.RestrictedFeatureTiles(ctx)
	if err != nil {
		s.Fatal("Failed to get the restricted tiles param: ", err)
	}

	// Verify that the tiles are restricted on the lock screen.
	for _, tile := range restrictedTiles {
		restricted, err := quicksettings.TileRestricted(ctx, tconn, tile)
		if err != nil {
			s.Fatalf("Failed to check restricted status of tile %v: %v", tile, err)
		}
		if !restricted {
			s.Errorf("Tile setting %v not restricted: %v", tile, err)
		}
	}

	hasBattery := s.Param().(bool)

	// Get the common Quick Settings elements to verify.
	checkNodes, err := quicksettings.CommonElements(ctx, tconn, hasBattery, true /* isLockedScreen */)
	if err != nil {
		s.Fatal("Failed to get the params in LockedScreen: ", err)
	}

	// Loop through all the Quick Settings nodes of locked screen and verify if they exist.
	ui := uiauto.New(tconn)
	for node, finder := range checkNodes {
		if err := ui.WaitUntilExists(finder)(ctx); err != nil {
			s.Fatalf("Failed to wait for %v node to exist: %v", node, err)
		}
		shown, err := ui.IsNodeFound(ctx, finder)
		if err != nil {
			s.Fatalf("Failed to find %v node: %v", node, err)
		}
		if !shown {
			s.Errorf("%v node was not shown in the UI", node)
		}
	}
}
