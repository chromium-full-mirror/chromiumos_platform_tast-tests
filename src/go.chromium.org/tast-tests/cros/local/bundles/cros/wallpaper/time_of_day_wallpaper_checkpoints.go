// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/media/imgcmp"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TimeOfDayWallpaperCheckpoints,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify a time of day wallpaper changes based on scheduled checkpoints",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"jasontt@google.com",
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

func TimeOfDayWallpaperCheckpoints(ctx context.Context, s *testing.State) {
	const (
		collection       = constants.DawnToDarkCollection
		image            = constants.EarthFlowImage
		colorDiffPercent = 5.0
	)
	var darkSkyColor = color.RGBA{5, 0, 20, 255}
	var lightSkyColor = color.RGBA{57, 139, 228, 255}

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

	if err := uiauto.Combine(fmt.Sprintf("Change the wallpaper to %s %s", collection, image),
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, collection),
		wallpaper.SelectImage(ui, image),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(image)),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", collection, image, err)
	}

	if err := uiauto.Combine("Enable dark mode",
		personalization.NavigateHome(ui),
		personalization.ToggleDarkMode(ui),
		wallpaper.MinimizeWallpaperPicker(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable dark mode: ", err)
	}

	// Take a screenshot of the dark mode wallpaper.
	darkScreenshot, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to grab dark screenshot: ", err)
	}

	darkColor := extractScreenShotColor(darkScreenshot)
	if err := validateColorDiff(darkColor, darkSkyColor, colorDiffPercent); err != nil {
		s.Fatal("Failed to validate that dark colors are the same: ", err)
	}

	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui),
		wallpaper.MinimizeWallpaperPicker(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	// Take a screenshot of the light mode wallpaper.
	lightScreenshot, err := screenshot.GrabScreenshot(ctx, cr)
	if err != nil {
		s.Fatal("Failed to grab light screenshot: ", err)
	}

	lightColor := extractScreenShotColor(lightScreenshot)
	if err := validateColorDiff(lightColor, lightSkyColor, colorDiffPercent); err != nil {
		s.Fatal("Failed to validate that colors are the same: ", err)
	}

	// Verify that the wallpaper has indeed changed.
	const expectedPercent = 90
	if err := wallpaper.ValidateDiff(darkScreenshot, lightScreenshot, expectedPercent); err != nil {
		lightScreenshotPath := filepath.Join(s.OutDir(), "light_screenshot.png")
		darkScreenshotPath := filepath.Join(s.OutDir(), "dark_screenshot.png")
		if err := imgcmp.DumpImageToPNG(ctx, &lightScreenshot, lightScreenshotPath); err != nil {
			s.Errorf("Failed to dump image to %s: %v", lightScreenshotPath, err)
		}
		if err := imgcmp.DumpImageToPNG(ctx, &darkScreenshot, darkScreenshotPath); err != nil {
			s.Errorf("Failed to dump image to %s: %v", darkScreenshotPath, err)
		}
		s.Fatal("Failed to validate wallpaper difference: ", err)
	}
}

// extractScreenShotColor extracts the color around the upper left position of the image.
func extractScreenShotColor(screenshot image.Image) color.Color {
	bounds := screenshot.Bounds()
	offsetX := bounds.Dx() / 4
	offsetY := bounds.Dy() / 8
	upperLeft := image.Point{bounds.Min.X + offsetX, bounds.Min.Y + offsetY}
	color := screenshot.At(upperLeft.X, upperLeft.Y)
	return color
}

// validateColorDiff checks the diff percentage between 2 colors and returns error if
// the percentage is more than expectedPercent.
func validateColorDiff(color1, color2 color.Color, expectedPercent float64) error {
	r1, g1, b1, _ := color1.RGBA()
	r2, g2, b2, _ := color2.RGBA()
	difference := math.Sqrt(math.Pow(float64(r1-r2), 2) + math.Pow(float64(g1-g2), 2) + math.Pow(float64(b1-b2), 2))
	percentage := difference / 255.0 * 100.0
	if percentage > expectedPercent {
		return errors.Errorf("unexpected percentage: got %v%%; more than %v%%", percentage, expectedPercent)
	}
	return nil
}
