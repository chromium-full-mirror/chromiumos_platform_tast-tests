// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/media/imgcmp"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type dailyRefreshGooglePhotosWallpaperParams struct {
	album    string
	isShared bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DailyRefreshGooglePhotosWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting Google Photos wallpapers as daily refresh source",
		Contacts: []string{
			"assistive-eng@google.com",
			"xiaohuic@google.com",
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
		Timeout:      5 * time.Minute,
		Fixture:      "personalizationWithGooglePhotosWallpaper",
		Params: []testing.Param{{
			Name: "from_album",
			Val: dailyRefreshGooglePhotosWallpaperParams{
				album:    constants.GooglePhotosWallpaperAlbum,
				isShared: false,
			},
		}, {
			Name: "from_shared_album",
			Val: dailyRefreshGooglePhotosWallpaperParams{
				album:    constants.GooglePhotosWallpaperSharedAlbum,
				isShared: true,
			},
		}},
	})
}

func DailyRefreshGooglePhotosWallpaper(ctx context.Context, s *testing.State) {
	// Setting Google Photos wallpapers requires that Chrome be logged in with
	// a user from an account pool which has been preconditioned to have a
	// Google Photos library with specific photos/albums present. Note that sync
	// is disabled to prevent flakiness caused by wallpaper cross device sync.

	cr := s.FixtValue().(*chrome.Chrome)

	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Force Chrome to be in clamshell mode to make sure the wallpaper view is
	// clearly visible for us to compare it with an expected RGBA color.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(ctx)

	// The test has a dependency on network speed, so we give `uiauto.Context`
	// ample time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	album := s.Param().(dailyRefreshGooglePhotosWallpaperParams).album
	isShared := s.Param().(dailyRefreshGooglePhotosWallpaperParams).isShared

	// Take a screenshot of the current wallpaper.
	screenshot1, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	if err := uiauto.Combine("Open Google Photos album",
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.GooglePhotosWallpaperCollection),
		ui.DoDefault(constants.GooglePhotosWallpaperAlbumsButton),
		wallpaper.SelectGooglePhotosAlbum(ui, album),
	)(ctx); err != nil {
		s.Fatal("Failed to open Google Photos album: ", err)
	}

	if err := enableGooglePhotosDailyRefresh(ctx, ui, isShared); err != nil {
		s.Fatal("Failed to enable daily refresh: ", err)
	}
	wallpaper.MinimizeWallpaperPicker(ui)

	// Take a screenshot of the current wallpaper.
	screenshot2, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	// Verify that the wallpaper has indeed changed.
	const expectedPercent = 90
	if err = wallpaper.ValidateDiff(screenshot1, screenshot2, expectedPercent); err != nil {
		screenshot1Path := filepath.Join(s.OutDir(), "screenshot_1.png")
		screenshot2Path := filepath.Join(s.OutDir(), "screenshot_2.png")
		if err := imgcmp.DumpImageToPNG(ctx, &screenshot1, screenshot1Path); err != nil {
			s.Errorf("Failed to dump image to %s: %v", screenshot1Path, err)
		}
		if err := imgcmp.DumpImageToPNG(ctx, &screenshot2, screenshot2Path); err != nil {
			s.Errorf("Failed to dump image to %s: %v", screenshot2Path, err)
		}
		s.Fatal("Failed to validate wallpaper difference: ", err)
	}

	if err := uiauto.Combine("Manually refresh and minimize wallpaper picker",
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.GooglePhotosWallpaperCollection),
		ui.DoDefault(constants.GooglePhotosWallpaperAlbumsButton),
		wallpaper.SelectGooglePhotosAlbum(ui, album),
		ui.DoDefault(constants.RefreshButton),

		// NOTE: The refresh button will be hidden while updating the wallpaper so
		// use its reappearance as a proxy to know when the wallpaper has finished
		// updating.
		ui.WaitUntilGone(constants.RefreshButton),
		ui.WaitUntilExists(constants.RefreshButton),

		wallpaper.MinimizeWallpaperPicker(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to manually refresh: ", err)
	}

	// Take a screenshot of the current wallpaper.
	screenshot3, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	// Verify that the wallpaper has indeed changed.
	if err = wallpaper.ValidateDiff(screenshot2, screenshot3, expectedPercent); err != nil {
		screenshot3Path := filepath.Join(s.OutDir(), "screenshot_3.png")
		if err := imgcmp.DumpImageToPNG(ctx, &screenshot3, screenshot3Path); err != nil {
			s.Errorf("Failed to dump image to %s: %v", screenshot3Path, err)
		}
		s.Fatal("Failed to validate wallpaper difference: ", err)
	}
}

// enableGooglePhotosDailyRefresh enables daily refresh in a Google Photos album.
func enableGooglePhotosDailyRefresh(ctx context.Context, ui *uiauto.Context, isShared bool) error {
	if err := ui.DoDefault(constants.ChangeDailyButton)(ctx); err != nil {
		return err
	}
	if isShared {
		if err := uiauto.Combine("Close pop-up confirmation dialog to proceed",
			ui.WaitUntilExists(constants.DialogTitle),
			ui.WaitUntilExists(constants.ProceedButton),
			ui.DoDefault(constants.ProceedButton),
		)(ctx); err != nil {
			return err
		}
	}
	return ui.WaitUntilExists(constants.RefreshButton)(ctx)
}
