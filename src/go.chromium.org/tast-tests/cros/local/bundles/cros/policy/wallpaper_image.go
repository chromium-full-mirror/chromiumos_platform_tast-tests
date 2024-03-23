// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"image/color"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/imagehelpers"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/externaldata"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WallpaperImage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Behavior of WallpaperImage policy, set the policy to a monochromatic wallpaper then take a screenshot of the desktop wallpaper and check the pixels percentage",
		Contacts: []string{
			"assistive-eng@google.com",
			"mohamedaomar@google.com", // Test author
		},
		BugComponent: "b:1006527", // ChromeOS > Software > Personalization
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Data:         []string{"wallpaper_image.jpeg"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WallpaperImage{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// WallpaperImage tests the WallpaperImage policy.
func WallpaperImage(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Check if the current device is in tablet mode.
	tablet, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to check tablet mode: ", err)
	}

	red := color.RGBA{255, 0, 0, 255}
	expectedPercent := 85
	// The background color in tablets is a bit different.
	// Since the launcher is always open in tablet mode and the apps icons took some space, the expected percentage is reduce to 70%.
	if tablet {
		red = color.RGBA{165, 13, 14, 255}
		expectedPercent = 70
	}

	// Open a keyboard device.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer kb.Close(ctx)

	eds, err := externaldata.NewServer(ctx)
	if err != nil {
		s.Fatal("Failed to create server: ", err)
	}
	defer eds.Stop(ctx)

	jpegBytes, err := imagehelpers.GetJPEGBytesFromFilePath(s.DataPath("wallpaper_image.jpeg"))
	if err != nil {
		s.Fatal("Failed to read wallpaper image: ", err)
	}
	iurl, ihash := eds.ServePolicyData(jpegBytes)

	for _, param := range []struct {
		name                string
		wantChangeWallpaper bool                   // wantChangeWallpaper is the a flag to check if it's allowed to change the wallpaper.
		wantImageCheck      bool                   // wantImageCheck is a flag to check the image pixels.
		value               *policy.WallpaperImage // value is the value of the policy.
	}{
		{
			name:                "nonempty",
			wantChangeWallpaper: false,
			wantImageCheck:      true,
			value:               &policy.WallpaperImage{Val: &policy.WallpaperImageValue{Url: iurl, Hash: ihash}},
		},
		{
			name:                "unset",
			wantChangeWallpaper: true,
			wantImageCheck:      false,
			value:               &policy.WallpaperImage{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			if param.wantImageCheck {
				// Check red percentage of the desktop.
				if err := wallpaper.ValidateBackground(cr, red, expectedPercent)(ctx); err != nil {
					s.Error("Failed to validate wallpaper: ", err)
				}
			}

			ui := uiauto.New(tconn)
			if err := personalization.OpenPersonalizationHub(ui)(ctx); err != nil {
				s.Error("Failed to open the personalization hub: ", err)
			}
			if param.wantChangeWallpaper {
				if err := ui.WaitUntilExists(personalization.ChangeWallpaperButton)(ctx); err != nil {
					s.Fatal("Failed to find change wallpaper button when it must exist: ", err)
				}
			} else {
				if err := ui.Gone(personalization.ChangeWallpaperButton)(ctx); err != nil {
					s.Fatal("Failed to ensure that change wallpaper button is gone: ", err)
				}
			}
		})
	}
}
