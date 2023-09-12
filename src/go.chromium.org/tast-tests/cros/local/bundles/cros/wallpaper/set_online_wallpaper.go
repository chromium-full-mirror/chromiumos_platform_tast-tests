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
		Func:         SetOnlineWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting online wallpapers in the new wallpaper app",
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

func SetOnlineWallpaper(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Force Chrome to be in clamshell mode to make sure wallpaper preview is not
	// enabled.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", constants.CityscapesCollection, constants.CityscapesImage),
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.CityscapesCollection),
		wallpaper.SelectImage(ui, constants.CityscapesImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.CityscapesImage)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.CityscapesCollection, constants.CityscapesImage, err)
	}

	// Navigate back to collection view by clicking on the back arrow in breadcrumb.
	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", constants.ImaginaryCollection, constants.ImaginaryImage),
		wallpaper.BackToWallpaper(ui),
		wallpaper.SelectCollection(ui, constants.ImaginaryCollection),
		wallpaper.SelectImage(ui, constants.ImaginaryImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.ImaginaryImage)))(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.ImaginaryCollection, constants.ImaginaryImage, err)
	}
}
