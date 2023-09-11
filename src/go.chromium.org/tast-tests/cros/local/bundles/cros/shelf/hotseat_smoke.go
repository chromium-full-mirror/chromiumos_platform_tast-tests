// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shelf

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HotseatSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the basic features of hotseat",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > System UI Surfaces > Shelf
		BugComponent: "b:1288352",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-a64ba692-0294-456d-a714-5c4226b9c93c",
		}},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.SkipOnModel("magister")),
		Pre:          chrome.LoggedIn(),
	})
}

// HotseatSmoke tests the basic features of hotseat.
func HotseatSmoke(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Verify that hotseat is shown after switching to tablet mode.
	{
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
		if err != nil {
			s.Fatal("Failed to enter tablet mode: ", err)
		}
		defer cleanup(ctx)

		if err := ash.WaitForHotseatAnimatingToIdealState(ctx, tconn, ash.ShelfShownHomeLauncher); err != nil {
			s.Fatal("Failed to show the shelf in tablet homelauncher: ", err)
		}
	}

	// Verify that hotseat is hidden after activating a window. Then it should be extended after gesture swipe.
	{
		const numWindows = 1
		if err := ash.CreateWindows(ctx, tconn, cr, ui.PerftestURL, numWindows); err != nil {
			s.Fatal("Failed to open browser windows: ", err)
		}

		tc, err := pointer.NewTouchController(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create the touch controller: ", err)
		}
		defer tc.Close(ctx)

		if err := ash.SwipeUpHotseatAndWaitForCompletion(ctx, tconn, tc.EventWriter(), tc.TouchCoordConverter()); err != nil {
			s.Fatal("Failed to swipe up the hotseat: ", err)
		}
	}

	// Verify that hotseat is shown after switching to clamshell mode.
	{
		if err := ash.SetTabletModeEnabled(ctx, tconn, false); err != nil {
			s.Fatal("Failed to enter clamshell mode: ", err)
		}

		if err := ash.WaitForHotseatAnimatingToIdealState(ctx, tconn, ash.ShelfShownClamShell); err != nil {
			s.Fatal("Failed to show the shelf after switching to clamshell: ", err)
		}
	}
}
