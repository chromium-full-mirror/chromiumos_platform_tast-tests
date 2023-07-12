// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/launcher/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	localPicturePath = "/home/chronos/user/MyFiles/Downloads/search_local_images.png"
	localPictureName = "search_local_images.png"
)

type testParam struct {
	TabletMode     bool
	Query          string
	ExpectedResult string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchLocalImage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that use local image search with different feature flags and search for a local image",
		Contacts: []string{
			"ml-service-team@google.com",
			"dgrebenyuk@google.org",
			"ypitsishin@google.org",
		},
		BugComponent: "b:280365665",
		Attr:         []string{"group:launcher_search_quality_daily"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{localPictureName},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "search_by_filename_then_click",
				Val: testParam{
					TabletMode:     false,
					Query:          "local",
					ExpectedResult: "Thoughts",
				},
				Fixture: fixture.LauncherImageSearch,
			},
			{
				Name: "search_by_paper_then_click",
				Val: testParam{
					TabletMode:     false,
					Query:          "paper",
					ExpectedResult: "Thoughts",
				},
				Fixture: fixture.LauncherImageSearchIca,
			},
		},
	})
}

func SearchLocalImage(ctx context.Context, s *testing.State) {
	image, err := ioutil.ReadFile(s.DataPath(localPictureName))
	if err != nil {
		s.Fatal("Failed to read an image: ", err)
	}

	if err = ioutil.WriteFile(localPicturePath, image, 0644); err != nil {
		s.Fatal("Failed to create an image in MyFiles: ", err)
	}

	defer os.Remove(localPicturePath)

	tconn := s.FixtValue().(fixture.LauncherSearchFixtData).TestAPIConn
	kb := s.FixtValue().(fixture.LauncherSearchFixtData).Keyboard
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tabletMode := s.Param().(testParam).TabletMode

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	ui := uiauto.New(tconn)
	ud := uidetection.NewDefault(tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	query := s.Param().(testParam).Query
	expectedResult := s.Param().(testParam).ExpectedResult
	picturePreview := nodewith.Role(role.Button).HasClass("ImageButton").NameContaining("search_local_images")
	picture := nodewith.Role(role.Window).HasClass("WebContentsViewAura").NameContaining("search_local_images")

	if err := uiauto.Retry(2, uiauto.NamedCombine("Search for image",
		launcher.ClearSearchField(tconn, kb),
		launcher.Search(tconn, kb, query),
		launcher.WaitForResultWithCategory(tconn, launcher.SearchCategoryInfo{
			Category:  "Images",
			NeedRegex: false,
			Result:    "search_local_images",
		}),
		ui.LeftClickUntil(picturePreview, ui.WithTimeout(3*time.Second).WaitUntilExists(picture)),
		launcher.VerifyTextWithUIDetection(ud, expectedResult),
	))(ctx); err != nil {
		s.Fatal("Failed to search image: ", err)
	}
}
