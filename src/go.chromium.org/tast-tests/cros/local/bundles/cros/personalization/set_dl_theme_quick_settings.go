// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetDLThemeQuickSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting dark light theme from quick settings",
		Contacts: []string{
			"cros-personalization@google.com",
			"chromeos-consumer-engprod@google.com",
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
		Fixture:      personalization.ClamshellFixture,
	})
}

func SetDLThemeQuickSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Force Chrome to be in clamshell mode to make sure wallpaper preview is not
	// enabled.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// By default after log in, dark light mode is set as Auto in Personlization Hub.
	// Switch to Light Mode for the test. ToggleLightMode() won't fail even if Light
	// mode is already enabled.
	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	if err := uiauto.Combine(fmt.Sprintf("Change to light mode wallpaper %s %s", constants.ElementCollection, constants.LightElementImage),
		personalization.OpenWallpaperSubpage(ui),
		wallpaper.SelectCollection(ui, constants.ElementCollection),
		wallpaper.SelectImage(ui, constants.LightElementImage),
		ui.WaitUntilExists(wallpaper.CurrentWallpaperWithSpecificNameFinder(constants.LightElementImage)),
		wallpaper.CloseWallpaperPicker(),
	)(ctx); err != nil {
		s.Fatalf("Failed to validate selected wallpaper %s %s: %v", constants.ElementCollection, constants.LightElementImage, err)
	}

	if err := toggleDarkThemeFromQuickSettings(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to turn on dark theme in Quick Settings: ", err)
	}

	if err := validateDLWallpaper(ctx, ui, constants.DarkElementImage); err != nil {
		s.Fatalf("Failed to change to dark mode wallpaper %v: %v", constants.DarkElementImage, err)
	}

	if err := toggleDarkThemeFromQuickSettings(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to turn off dark theme in Quick Settings: ", err)
	}

	if err := validateDLWallpaper(ctx, ui, constants.LightElementImage); err != nil {
		s.Fatalf("Failed to change to light mode wallpaper %v: %v", constants.LightElementImage, err)
	}
}

func toggleDarkThemeFromQuickSettings(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show quick settings")
	}
	defer quicksettings.Hide(ctx, tconn)

	if err := uiauto.Combine("Toggle Dark theme",
		ui.LeftClick(quicksettings.DisplaySettingsButton),
		ui.LeftClick(quicksettings.FeatureTileDarkTheme),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle Dark theme")
	}
	return nil
}

func validateDLWallpaper(ctx context.Context, ui *uiauto.Context, image string) error {
	if err := wallpaper.OpenWallpaperPicker(ui)(ctx); err != nil {
		return errors.Wrap(err, "failed to open wallpaper picker")
	}

	currentWallpaper, err := wallpaper.CurrentWallpaper(ctx, ui)
	if err != nil {
		return err
	}
	if !strings.Contains(currentWallpaper, image) {
		return errors.Errorf("current wallpaper - %v is not as expected - %v", currentWallpaper, image)
	}

	if err := wallpaper.CloseWallpaperPicker()(ctx); err != nil {
		return errors.Wrap(err, "failed to close wallpaper picker")
	}

	return nil
}
