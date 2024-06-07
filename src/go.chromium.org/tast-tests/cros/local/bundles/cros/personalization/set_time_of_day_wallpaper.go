// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package personalization supports interaction with ChromeOS personalization app.
package personalization

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetTimeOfDayWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting a time of day wallpaper",
		Contacts: []string{
			"cros-personalization@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:cbx", "cbx_feature_enabled", "cbx_stable"},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      "personalizationWithTimeOfDayFeatureClamshell",
	})
}

func SetTimeOfDayWallpaper(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

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

	if err := uiauto.Combine("Enable auto mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleAutoMode(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable auto mode: ", err)
	}

	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", constants.DawnToDarkCollection, constants.EarthFlowImage),
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.DawnToDarkCollection),
		wallpaper.SelectImage(ui, constants.EarthFlowImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.EarthFlowImage)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.DawnToDarkCollection, constants.EarthFlowImage, err)
	}

	// Navigate back to collection view by clicking on the back arrow in breadcrumb.
	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", constants.DawnToDarkCollection, constants.CloudFlowImage),
		wallpaper.BackToWallpaper(ui),
		wallpaper.SelectCollection(ui, constants.DawnToDarkCollection),
		wallpaper.SelectImage(ui, constants.CloudFlowImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.CloudFlowImage)))(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.DawnToDarkCollection, constants.CloudFlowImage, err)
	}
}
