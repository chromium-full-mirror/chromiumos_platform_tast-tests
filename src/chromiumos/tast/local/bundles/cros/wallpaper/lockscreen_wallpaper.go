// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"image/color"
	"math"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/fsutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/lockscreen"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/local/wallpaper"
	"chromiumos/tast/local/wallpaper/constants"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LockscreenWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test verifying wallpaper on lock screen",
		Contacts: []string{
			"jasontt@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{constants.LockscreenWallpaperFileName},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func LockscreenWallpaper(ctx context.Context, s *testing.State) {
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

	filePath, err := wallpaper.LocalImageDownloadPath(ctx, cr.NormalizedUser(), constants.LockscreenWallpaperFileName)
	if err != nil {
		s.Fatalf("Failed to get path for file %v, %v: ", constants.LockscreenWallpaperFileName, err)
	}

	if err := fsutil.CopyFile(s.DataPath(constants.LockscreenWallpaperFileName), filePath); err != nil {
		s.Fatalf("Failed to copy %s to %s: %v", constants.LockscreenWallpaperFileName, filePath, err)
	}

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := uiauto.Combine("Set a new custom wallpaper and minimize wallpaper picker",
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.LocalWallpaperCollection),
		wallpaper.SelectImage(ui, constants.LockscreenWallpaperFileName),
		wallpaper.MinimizeWallpaperPicker(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to set new wallpaper: ", err)
	}

	// Lock the screen
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for the screen to be locked failed: %v (last status %+v)", err, st)
	}
	// Unlock the screen to ensure subsequent tests aren't affected by the screen remaining locked.
	// TODO(b/187794615): Remove once chrome.go has a way to clean up the lock screen state.
	defer func() {
		if err := lockscreen.Unlock(ctx, tconn); err != nil {
			s.Fatal("Failed to unlock the screen: ", err)
		}
	}()

	// Take a screenshot of the lock screen.
	lockScreenshot, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to grab screenshot: ", err)
	}

	red := color.RGBA{255, 0, 0, 255}
	blue := color.RGBA{0, 255, 0, 255}
	green := color.RGBA{0, 0, 255, 255}
	threshold := float64(10)

	redDistance := float64(wallpaper.ColorDistance(lockScreenshot.At(50, 50), red))
	blueDistance := float64(wallpaper.ColorDistance(lockScreenshot.At(lockScreenshot.Bounds().Dx()-50, 50), blue))
	greenDistance := float64(wallpaper.ColorDistance(lockScreenshot.At(50, lockScreenshot.Bounds().Dy()-50), green))

	// The shield is applied evenly so we expect the color distance of the original color and the shielded color to be relatively the same for all three zones.
	if math.Abs(redDistance-blueDistance) > threshold || math.Abs(redDistance-greenDistance) > threshold || math.Abs(blueDistance-greenDistance) > threshold {
		s.Fatalf("Failed to verify wallpaper on lockscreen: redDistance - %v, blueDistance - %v, greenDistance - %v", redDistance, blueDistance, greenDistance)
	}
}
