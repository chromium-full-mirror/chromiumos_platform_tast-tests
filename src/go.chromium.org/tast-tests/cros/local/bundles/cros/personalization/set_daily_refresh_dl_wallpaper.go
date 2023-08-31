// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetDailyRefreshDLWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting D/L wallpapers daily refresh in the personalization hub app",
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

func SetDailyRefreshDLWallpaper(ctx context.Context, s *testing.State) {
	// Dark mode wallpaper title includes "Dark" word but light mode wallpaper title doesn't.
	darkRegex := regexp.MustCompile(`Currently set Daily Refresh .*\sDark\s.*`)

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

	if err := uiauto.Combine("select Elements collection and enable daily refresh",
		personalization.OpenPersonalizationHub(ui),
		personalization.OpenWallpaperSubpage(ui),
		wallpaper.SelectCollection(ui, constants.ElementCollection),
		ui.LeftClick(constants.ChangeDailyButton),
		ui.WaitUntilExists(constants.RefreshButton),
	)(ctx); err != nil {
		s.Fatal("Failed to enable daily refresh: ", err)
	}

	darkWallpaper := nodewith.Role(role.Heading).HasClass("preview-text-container").NameRegex(darkRegex)
	if err := uiauto.Combine("enable dark mode and validate daily refresh wallpaper",
		personalization.NavigateHome(ui),
		personalization.ToggleDarkMode(ui),
		personalization.OpenWallpaperSubpage(ui),
		ui.WaitUntilExists(darkWallpaper),
	)(ctx); err != nil {
		s.Fatal("Failed to validate daily refresh dark mode wallpaper: ", err)
	}

	dailyRefreshWallpaper := nodewith.Role(role.Heading).HasClass("preview-text-container").NameStartingWith("Currently set Daily Refresh")
	if err := uiauto.Combine("enable light mode and validate daily refresh wallpaper",
		personalization.NavigateHome(ui),
		personalization.ToggleLightMode(ui),
		personalization.OpenWallpaperSubpage(ui),
		ui.WaitUntilExists(dailyRefreshWallpaper),
	)(ctx); err != nil {
		s.Fatal("Failed to validate daily refresh wallpaper: ", err)
	}

	darkWallpaperFound, err := ui.IsNodeFound(ctx, darkWallpaper)
	if err != nil {
		s.Fatal("Failed to search for darkWallpaper node: ", err)
	}
	if darkWallpaperFound {
		s.Fatal("Failed to validate daily refresh light mode wallpaper, daily refresh wallpaper is still in dark mode")
	}
}
