// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wallpapergooglephotos contains helpers to verify the successful behavior
// of the personalization application wallpaper page integration with google
// photos.
package wallpapergooglephotos

import (
	"context"
	"net/http/httptest"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/errors"
)

const (
	// EnabledHashCode is the hashcode of the network annotation tag with
	// id: wallpaper_google_photos_enabled.
	EnabledHashCode = "50590711"
	// AlbumsHashCode is the hashcode of the network annotation tag with
	// id: wallpaper_google_photos_albums.
	AlbumsHashCode = "81192642"
	// PhotosHashCode is the hashcode of the network annotation tag with
	// id: wallpaper_google_photos_photos.
	PhotosHashCode = "93311068"
)

// testCase defines test expectations based on the policy value.
type testCase struct {
	Name                                  string
	ShouldGooglePhotosCollectionBeEnabled bool
	ShouldFindAnnotations                 bool
	Policy                                *policy.WallpaperGooglePhotosIntegrationEnabled
}

// TestCases returns the list of testCase objects on which
// WallpaperGooglePhotosIntegrationEnabled policy is tested.
func TestCases() []testCase {
	return []testCase{
		{
			Name:                                  "disabled",
			ShouldGooglePhotosCollectionBeEnabled: false,
			ShouldFindAnnotations:                 false,
			Policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Val: false},
		},
		{
			Name:                                  "unset",
			ShouldGooglePhotosCollectionBeEnabled: true,
			ShouldFindAnnotations:                 true,
			Policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Stat: policy.StatusUnset},
		},
		{
			Name:                                  "enabled",
			ShouldGooglePhotosCollectionBeEnabled: true,
			ShouldFindAnnotations:                 true,
			Policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Val: true},
		},
	}
}

// TriggerWallpaperGooglePhotosIntegration verifies that launching the
// wallpaper google photos collection from the personalization app
// works as expected.
func TriggerWallpaperGooglePhotosIntegration(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := TestCases()[paramIndex]

	windows, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get windows")
	}

	// Minimize all open windows to allow the OpenPersonalizationHub
	// method to right-click the desktop.
	for _, window := range windows {
		if _, err := ash.SetWindowState(ctx, tconn, window.ID, ash.WMEventMinimize, false /* waitForStateChange */); err != nil {
			return errors.Wrap(err, "failed to minimize browser window")
		}
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("open wallpaper page of personalization hub",
		personalization.OpenPersonalizationHub(ui),
		personalization.OpenWallpaperSubpage(ui),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open wallpaper subpage of personalization hub")
	}

	// Loaded collections have names like name=20 Images.
	loadedCollections := nodewith.NameRegex(regexp.MustCompile(`.*\d+\s[iI]mages`)).First()
	googlePhotosCollection := loadedCollections.NameStartingWith(constants.GooglePhotosWallpaperCollection)
	if err := ui.WaitUntilExists(googlePhotosCollection)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for Google Photos wallpaper collection")
	}

	googlePhotosLink := nodewith.Name("Google Photos").Role(role.ListBoxOption)
	info, err := ui.Info(ctx, googlePhotosLink)
	if err != nil {
		return errors.Wrap(err, "failed to get google photos collection link info")
	}

	isGooglePhotosCollectionEnabled := info.HTMLAttributes["aria-disabled"] != "true"

	if err := wallpaper.SelectCollection(ui, constants.GooglePhotosWallpaperCollection)(ctx); err != nil {
		return errors.Wrap(err, "failed to select google photos item")
	}

	if param.ShouldGooglePhotosCollectionBeEnabled != isGooglePhotosCollectionEnabled {
		return errors.Errorf("unexpected Google Photos collection enabled state: got %t expected %t", isGooglePhotosCollectionEnabled, param.ShouldGooglePhotosCollectionBeEnabled)
	}

	return nil
}
