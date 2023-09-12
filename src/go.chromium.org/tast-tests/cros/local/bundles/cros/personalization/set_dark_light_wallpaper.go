// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetDarkLightWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting D/L wallpapers in the personalization hub app",
		Contacts: []string{
			"assistive-eng@google.com",
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-47bb4826-69df-4c03-aaf2-e9a8a0f0f636",
		}},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Fixture:      "personalizationWithClamshell",
	})
}

func SetDarkLightWallpaper(ctx context.Context, s *testing.State) {
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

	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", constants.ElementCollection, constants.DarkElementImage),
		personalization.OpenWallpaperSubpage(ui),
		wallpaper.SelectCollection(ui, constants.ElementCollection),
		wallpaper.SelectImage(ui, constants.DarkElementImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.DarkElementImage)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.ElementCollection, constants.DarkElementImage, err)
	}

	if err := uiauto.Combine("Enable light mode",
		personalization.NavigateHome(ui),
		personalization.ToggleLightMode(ui))(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	if err := uiauto.Combine(fmt.Sprintf("Validate currently set wallpaper changed to %s %s", constants.ElementCollection, constants.LightElementImage),
		personalization.OpenWallpaperSubpage(ui),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.LightElementImage)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.ElementCollection, constants.LightElementImage, err)
	}
}
