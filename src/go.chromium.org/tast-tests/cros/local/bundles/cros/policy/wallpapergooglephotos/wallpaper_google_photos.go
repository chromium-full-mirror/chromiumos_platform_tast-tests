// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wallpapergooglephotos contains helpers to verify the successful behavior
// of the personalization application wallpaper page integration with google
// photos.
package wallpapergooglephotos

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
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
func TestCases() map[networkrequestmonitor.PolicySetting]testCase {
	return map[networkrequestmonitor.PolicySetting]testCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                                  "disabled",
			ShouldGooglePhotosCollectionBeEnabled: false,
			ShouldFindAnnotations:                 false,
			Policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Val: false},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                                  "unset",
			ShouldGooglePhotosCollectionBeEnabled: true,
			ShouldFindAnnotations:                 true,
			Policy:                                &policy.WallpaperGooglePhotosIntegrationEnabled{Stat: policy.StatusUnset},
		},
		networkrequestmonitor.PolicyEnabled: {
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
func TriggerWallpaperGooglePhotosIntegration(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	policyParam := TestCases()[params.PolicySetting]

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

	// Open the wallpaper subpage of the personalization app, and select the
	// Google Photos collection.
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("open google photos wallpaper collection in personalization hub",
		wallpaper.OpenWallpaperPicker(ui),
		wallpaper.SelectCollection(ui, constants.GooglePhotosWallpaperCollection),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open google photos wallpaper collection in personalization hub")
	}

	// If the Google Photos collection is enabled, there will now be a Google Photos
	// Link in the breadcrumbs of the page.
	googlePhotosBreadcrumbFound := true
	if err := ui.WithTimeout(2 * time.Second).WaitUntilExists(personalization.BreadcrumbNodeFinder(constants.GooglePhotosWallpaperCollection))(ctx); err != nil {
		if !nodewith.IsNodeNotFoundErr(err) {
			return errors.Wrap(err, "failure while trying to find Google Photos breadcrumb")
		}
		googlePhotosBreadcrumbFound = false
	}

	if policyParam.ShouldGooglePhotosCollectionBeEnabled != googlePhotosBreadcrumbFound {
		return errors.Errorf("unexpected Google Photos collection enabled state: got %t expected %t", googlePhotosBreadcrumbFound, policyParam.ShouldGooglePhotosCollectionBeEnabled)
	}

	return nil
}
