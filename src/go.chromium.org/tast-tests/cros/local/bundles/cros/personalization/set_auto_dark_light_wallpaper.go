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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/wallpaper"
	"go.chromium.org/tast-tests/cros/local/wallpaper/constants"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParams struct {
	darkModeEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetAutoDarkLightWallpaper,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting auto theme D/L wallpapers in the personalization hub app",
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
		Timeout:      3 * time.Minute,
		Fixture:      personalization.ClamshellFixture,
		Params: []testing.Param{
			{
				Name: "dark",
				Val: testParams{
					darkModeEnabled: true,
				},
			},
			{
				Name: "light",
				Val: testParams{
					darkModeEnabled: false,
				},
			},
		},
	})
}

func SetAutoDarkLightWallpaper(ctx context.Context, s *testing.State) {
	darkModeEnabled := s.Param().(testParams).darkModeEnabled
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

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

	if err := uiauto.Combine("Enable auto mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleAutoMode(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable auto mode: ", err)
	}

	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.forceAutoThemeMode)`, darkModeEnabled); err != nil {
		s.Fatalf("Failed to force auto theme mode for testing, dark mode enabled - %v: %v", darkModeEnabled, err)
	}

	// Open Elements collection.
	if err := uiauto.Combine(fmt.Sprintf("Select wallpaper from %v collection", constants.ElementCollection),
		personalization.OpenWallpaperSubpage(ui),
		wallpaper.SelectCollection(ui, constants.ElementCollection),
		ui.WaitUntilExists(personalization.BreadcrumbNodeFinder(constants.ElementCollection)),
	)(ctx); err != nil {
		s.Fatalf("Failed to select collection %v : %v", constants.ElementCollection, err)
	}

	if err := selectDLWallpaper(ctx, ui, constants.ElementCollection); err != nil {
		s.Fatalf("Failed to select an image in %v collection as wallpaper: %v", constants.ElementCollection, err)
	}

	currentWallpaper, err := wallpaper.CurrentWallpaper(ctx, ui)
	if err != nil {
		s.Fatal("Failed to get current wallpaper name: ", err)
	}

	if darkModeEnabled && !strings.Contains(currentWallpaper, "Dark") {
		s.Fatal("Failed to set wallpaper to dark mode, current wallpaper: ", currentWallpaper)
	}

	if !darkModeEnabled && !strings.Contains(currentWallpaper, "Light") {
		s.Fatal("Failed to set wallpaper to light mode, current wallpaper: ", currentWallpaper)
	}
}

func waitUntilSelected(ac *uiauto.Context, finder *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			nodeInfo, err := ac.Info(ctx, finder)
			if err != nil {
				return err
			}
			if !nodeInfo.Selected {
				return errors.Wrapf(err, "%v selected state is not %v", nodeInfo.Name, true)
			}
			return nil
		}, &testing.PollOptions{Timeout: 2 * time.Second})
	}
}

// selectDLWallpaper selects an image in a D/L collection and sets it as wallpaper.
func selectDLWallpaper(ctx context.Context, ui *uiauto.Context, collection string) error {
	allImages := nodewith.Role(role.ListBoxOption).Ancestor(nodewith.Role(role.Main).Name(collection))

	images, err := ui.NodesInfo(ctx, allImages)
	if err != nil {
		return errors.Wrapf(err, "failed to find images in %v collection", collection)
	}
	if len(images) < 8 {
		return errors.Errorf("at least 8 image options for %v collection expected", collection)
	}

	// Select 3rd image as wallpaper.
	imageFinder := allImages.Nth(2)

	imageInfo, err := ui.Info(ctx, imageFinder)
	if err != nil {
		return errors.Wrap(err, "failed to get info for 3rd Element image")
	}

	if err := uiauto.Combine("Click Element image",
		ui.WithTimeout(10*time.Second).LeftClickUntil(
			imageFinder,
			waitUntilSelected(ui, imageFinder),
		),
		ui.WaitUntilExists(nodewith.Role(role.Heading).NameContaining(imageInfo.Name)))(ctx); err != nil {
		return errors.Wrapf(err, "failed to click Element image %v", imageInfo.Name)
	}

	return nil
}
