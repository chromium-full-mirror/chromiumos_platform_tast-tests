// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetTimeOfDayWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting a time of day wallpaper",
		Contacts: []string{
			"assistive-eng@google.com",
			"jasontt@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model("vell")),
		Timeout:      5 * time.Minute,
		Fixture:      "personalizationWithTimeOfDayFeatureClamshell",
	})
}

func SetTimeOfDayWallpaper(ctx context.Context, s *testing.State) {
	const (
		collection  = constants.DawnToDarkCollection
		firstImage  = constants.EarthFlowImage
		secondImage = constants.CloudFlowImage
	)

	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := uiauto.Combine("Enable dark mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleDarkMode(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable dark mode: ", err)
	}

	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", collection, firstImage),
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, collection),
		wallpaper.SelectImage(ui, firstImage),
		wallpaper.ConfirmTimeOfDayWallpaper(ui),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(firstImage)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", collection, firstImage, err)
	}

	// Navigate back to collection view by clicking on the back arrow in breadcrumb.
	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", collection, secondImage),
		wallpaper.BackToWallpaper(ui),
		wallpaper.SelectCollection(ui, collection),
		wallpaper.SelectImage(ui, secondImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(secondImage)))(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", collection, secondImage, err)
	}
}
