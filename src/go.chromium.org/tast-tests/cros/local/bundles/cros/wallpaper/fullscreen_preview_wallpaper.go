// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FullscreenPreviewWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test full screen preview of local wallpapers in the new wallpaper app",
		Contacts: []string{
			"cros-personalization@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-5d88ff46-9f09-4e3c-aef2-df56852ead57",
		}},
		Data:         []string{constants.LocalWallpaperFilename},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      personalization.BaseFixture,
	})
}

func FullscreenPreviewWallpaper(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Force Chrome to be in tablet mode to trigger full screen preview flow.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	filePath, err := wallpaper.LocalImageDownloadPath(ctx, cr.NormalizedUser(), constants.LocalWallpaperFilename)
	if err != nil {
		s.Fatalf("Failed to get path for file %v, %v: ", constants.LocalWallpaperFilename, err)
	}

	if err := fsutil.CopyFile(s.DataPath(constants.LocalWallpaperFilename), filePath); err != nil {
		s.Fatalf("Could not copy %s to %s: %v", constants.LocalWallpaperFilename, filePath, err)
	}

	// Remove notifications otherwise can accidentally click on explore app notification.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := uiauto.Combine("preview and confirm online wallpaper",
		wallpaper.OpenWallpaperPicker(ui),
		previewOnlineWallpaper(ui, cr),
		wallpaper.ConfirmFullscreenPreview(ui),
		wallpaper.WaitForWallpaperWithName(ui, constants.YellowWallpaperName),
	)(ctx); err != nil {
		s.Fatal("Failed to select an online wallpaper: ", err)
	}

	if err := uiauto.Combine("preview and cancel local wallpaper",
		wallpaper.BackToWallpaper(ui),
		previewLocalWallpaper(ui, cr),
		wallpaper.CancelFullscreenPreview(ui),
		// Should revert to online wallpaper.
		wallpaper.WaitForWallpaperWithName(ui, constants.YellowWallpaperName),
	)(ctx); err != nil {
		s.Fatal("Failed to preview local wallpaper: ", err)
	}

	localWallpaperFilenameWithoutExtension := strings.TrimSuffix(constants.LocalWallpaperFilename, filepath.Ext(constants.LocalWallpaperFilename))

	if err := uiauto.Combine("preview and confirm local wallpaper",
		wallpaper.BackToWallpaper(ui),
		previewLocalWallpaper(ui, cr),
		wallpaper.ConfirmFullscreenPreview(ui),
		wallpaper.WaitForWallpaperWithName(ui, localWallpaperFilenameWithoutExtension),
	)(ctx); err != nil {
		s.Fatal("Failed to select a local wallpaper: ", err)
	}

	if err := uiauto.Combine("preview and cancel online wallpaper",
		wallpaper.BackToWallpaper(ui),
		previewOnlineWallpaper(ui, cr),
		wallpaper.CancelFullscreenPreview(ui),
		// Should revert to local wallpaper.
		wallpaper.WaitForWallpaperWithName(ui, localWallpaperFilenameWithoutExtension),
		wallpaper.CloseWallpaperPicker(),
	)(ctx); err != nil {
		s.Fatal("Failed to preview online wallpaper: ", err)
	}
}

func previewOnlineWallpaper(ui *uiauto.Context, cr *chrome.Chrome) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("preview online wallpaper %q", constants.YellowWallpaperName),
		wallpaper.SelectCollection(ui, constants.SolidColorsCollection),
		wallpaper.SelectImage(ui, constants.YellowWallpaperName),
		wallpaper.ValidateBackground(cr, constants.YellowWallpaperColor, 70))
}

func previewLocalWallpaper(ui *uiauto.Context, cr *chrome.Chrome) uiauto.Action {
	return uiauto.Combine("Preview custom wallpaper and click back",
		wallpaper.SelectCollection(ui, constants.LocalWallpaperCollection),
		wallpaper.SelectImage(ui, constants.LocalWallpaperFilename),
		wallpaper.ValidateBackground(cr, constants.LocalWallpaperColor, 70))
}
