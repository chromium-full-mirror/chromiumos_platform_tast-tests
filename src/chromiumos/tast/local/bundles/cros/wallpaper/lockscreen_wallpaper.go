// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/fsutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/lockscreen"
	"chromiumos/tast/local/personalization"
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
			"assistive-eng@google.com",
			"jasontt@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{constants.LockscreenWallpaperFileName},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "dark_mode",
			Val:  personalization.ToggleDarkMode,
		}, {
			Name: "light_mode",
			Val:  personalization.ToggleLightMode,
		}},
	})
}

// areColorChannelsClose returns true if `a` and `b` are less than 5% apart. Despite being uint32, both `a` and `b` must be in the range [0, 2^16 - 1] as required by `color.Color`.
func areColorChannelsClose(a, b uint32) bool {
	return (math.Abs(float64(a)-float64(b)) / 0xffff) < 0.05
}

// isFirstValueDominant returns true if x >> y and y ~= z. This is useful for determining if a color is visually dominated by one channel, ie red, green, or blue.
func isFirstValueDominant(x, y, z uint32) bool {
	return x > y && !areColorChannelsClose(x, y) && areColorChannelsClose(y, z)
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return isFirstValueDominant(r, g, b)
}

func isGreen(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return isFirstValueDominant(g, r, b)
}

func isBlue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return isFirstValueDominant(b, g, r)
}

func saveLockscreenJpg(outdir string, image image.Image) error {
	file, err := os.Create(filepath.Join(outdir, "lockscreen.jpg"))
	if err != nil {
		return errors.Wrap(err, "failed to create file lockscreen.jpg")
	}
	if err = png.Encode(file, image); err != nil {
		return errors.Wrap(err, "failed to write lockscreen.jpg")
	}
	return nil
}

// LockscreenWallpaper verifies that a reference red, green, blue wallpaper can be seen in blurred form when the screen is locked.
// TODO(b/264906039) update this test when DarkLightModeKMeansColor launches.
func LockscreenWallpaper(ctx context.Context, s *testing.State) {
	// Using fixture may leave the DUT in a locked state that affects the tests
	// that follow in the same fixture.
	cr, err := chrome.New(ctx, chrome.DisableFeatures("DarkLightModeKMeansColor"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

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

	// An action to toggle the theme to either light or dark mode.
	toggleTheme := s.Param().(func(*uiauto.Context) uiauto.Action)

	if err := uiauto.Combine("Set a new custom wallpaper and minimize wallpaper picker",
		personalization.OpenPersonalizationHub(ui),
		toggleTheme(ui),
		personalization.OpenWallpaperSubpage(ui),
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
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool {
		return st.Locked && st.ReadyForPassword && !st.WallpaperAnimating
	}, 30*time.Second); err != nil {
		s.Fatalf("Waiting for the screen to be locked failed: %v (last status %+v)", err, st)
	}

	// Take a screenshot of the lock screen.
	lockscreenImage, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to take lockscreen screenshot: ", err)
	}

	bounds := lockscreenImage.Bounds()
	offsetX := bounds.Dx() / 4
	offsetY := bounds.Dy() / 8
	upperLeft := image.Point{bounds.Min.X + offsetX, bounds.Min.Y + offsetY}
	upperRight := image.Point{bounds.Max.X - offsetX, upperLeft.Y}
	bottomCenter := image.Point{(bounds.Min.X + bounds.Max.X) / 2, bounds.Max.Y - offsetY}

	red := lockscreenImage.At(upperLeft.X, upperLeft.Y)
	green := lockscreenImage.At(upperRight.X, upperRight.Y)
	blue := lockscreenImage.At(bottomCenter.X, bottomCenter.Y)

	if !isRed(red) || !isGreen(green) || !isBlue(blue) {
		if err = saveLockscreenJpg(s.OutDir(), lockscreenImage); err != nil {
			s.Error("Failed to save debug lockscreen image: ", err)
		}
		s.Fatalf("Failed to verify wallpaper on lockscreen: red - %v, green - %v, blue - %v", red, green, blue)
	}
}
