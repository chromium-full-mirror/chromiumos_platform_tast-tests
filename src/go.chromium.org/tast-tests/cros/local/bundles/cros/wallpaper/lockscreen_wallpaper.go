// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LockscreenWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test verifying wallpaper on lock screen",
		Contacts: []string{
			"cros-personalization@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-2502e146-d4a3-4251-baba-d3cd24f18b45",
		}},
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

// RGBA values are in the range [0, 65536)
const (
	// The minimum amount that the primary color must be greater than secondary colors
	minimumPrimaryColorDiff uint32 = 10000
	// The maximum amount that secondary colors can differ
	maximumSecondaryColorDiff = 4000
	minimumAlpha              = 65000
)

func isPrimaryColor(primary, x, y, alpha uint32) bool {
	if alpha <= minimumAlpha {
		return false
	}

	if primary <= x || primary-x <= minimumPrimaryColorDiff {
		return false
	}

	if primary <= y || primary-y <= minimumPrimaryColorDiff {
		return false
	}

	return uint32(math.Abs(float64(x)-float64(y))) < maximumSecondaryColorDiff
}

// isRed tests that red is highest and {green,blue} are similar.
func isRed(c color.Color) bool {
	r, g, b, a := c.RGBA()
	return isPrimaryColor(r, g, b, a)
}

// isGreen tests that green is highest and {red, blue} are similar.
func isGreen(c color.Color) bool {
	r, g, b, a := c.RGBA()
	return isPrimaryColor(g, r, b, a)
}

// isBlue tests that blue is highest and {red, green} are similar.
func isBlue(c color.Color) bool {
	r, g, b, a := c.RGBA()
	return isPrimaryColor(b, r, g, a)
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

// printRGBA prints the values in range [0, 65536) rather than the color.Color default of [0, 255].
func printRGBA(c color.Color) string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("{%d, %d, %d, %d}", r, g, b, a)
}

// LockscreenWallpaper verifies that a reference red, green, blue wallpaper can be seen in blurred form when the screen is locked.
// The dominant extracted color is blue, so all colors will be shifted towards blue.
func LockscreenWallpaper(ctx context.Context, s *testing.State) {
	// Using fixture may leave the DUT in a locked state that affects the tests that follow in the same fixture.
	// Use `chrome.New` so that other tests are unaffected if this test fails.
	cr, err := chrome.New(ctx)
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

	// Lock the screen.
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

	// The test wallpaper is divided into three parts of red, green, or blue. All three are shifted towards blue because
	// blue is the largest region in the image, so system UI should do color extraction and shift the lockscreen towards blue.
	// Verify that a pixel sampled from the red area is red+blue, a pixel sampled from the green area is green+blue,
	// and a pixel sampled from the blue area is very blue.
	if !isRed(red) || !isGreen(green) || !isBlue(blue) {
		if err = saveLockscreenJpg(s.OutDir(), lockscreenImage); err != nil {
			s.Error("Failed to save debug lockscreen image: ", err)
		}
		s.Fatalf("Failed to verify wallpaper on lockscreen: red - %s, green - %s, blue - %s", printRGBA(red), printRGBA(green), printRGBA(blue))
	}
}
