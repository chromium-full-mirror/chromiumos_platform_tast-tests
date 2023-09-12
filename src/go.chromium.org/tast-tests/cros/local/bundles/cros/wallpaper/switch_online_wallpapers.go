// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SwitchOnlineWallpapers,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test quickly switching online wallpapers in the new wallpaper app",
		Contacts: []string{
			"assistive-eng@google.com",
			"jasontt@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-2502e146-d4a3-4251-baba-d3cd24f18b45",
		}},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

// SwitchOnlineWallpapers tests the flow of rapidly switching online wallpapers from the same
// collection and make sure the correct one is displayed.
func SwitchOnlineWallpapers(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Force Chrome to be in clamshell mode to make sure wallpaper view is clearly
	// visible for us to compare it with the given rgba color.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample time to
	// wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := wallpaper.OpenWallpaperPicker(ui)(ctx); err != nil {
		s.Fatal("Failed to open wallpaper picker: ", err)
	}

	if err := wallpaper.SelectCollectionWithScrolling(ctx, ui, constants.SolidColorsCollection); err != nil {
		s.Fatalf("Failed to select collection %q: %v", constants.SolidColorsCollection, err)
	}

	// Make sure yellow is last in the slice. We will be comparing the background wallpaper
	// with the given rgba color.
	for _, image := range []string{"Light Blue", "Google Green", "Google Yellow", constants.YellowWallpaperName} {
		// Select the given image, then check the selected wallpaper loads.
		if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %q", image),
			wallpaper.SelectImage(ui, image),
			ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(image)))(ctx); err != nil {
			s.Fatalf("Failed to select and validate wallpaper %q: %v", image, err)
		}
	}

	if err := wallpaper.MinimizeWallpaperPicker(ui)(ctx); err != nil {
		s.Fatal("Failed to minimize wallpaper picker: ", err)
	}

	const expectedPercent = 90
	if err := wallpaper.ValidateBackground(cr, constants.YellowWallpaperColor, expectedPercent)(ctx); err != nil {
		s.Error("Failed to validate wallpaper background: ", err)
	}
}
