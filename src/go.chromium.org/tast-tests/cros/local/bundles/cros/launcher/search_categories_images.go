// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/launcher/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/launcher/util"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const localPictureName = util.ImageSearchLocalPictureName

var queryCategoryInfo = launcher.SearchCategoryInfo{
	Category:  "Images",
	NeedRegex: false,
	Result:    "search_local_image",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchCategoriesImages,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that removing images item from search catetories will not show image search result",
		Contacts: []string{
			"launcher-search-notify@google.com",
			"chenjih@google.com",
			"ypitsishin@google.com",
		},
		BugComponent: "b:1281467",
		SoftwareDeps: []string{"chrome", "ondevice_image_content_annotation"},
		Data:         []string{launcher.ImageSearchPowerTestPictureName},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.LauncherImageSearchOcr,
		Attr: []string{"group:cbx",
			"cbx_feature_enabled",
			"cbx_unstable"},
		TestBedDeps: []string{tbdep.Cbx(true)},
		Params: []testing.Param{
			{
				Val: util.ImageSearchTestParam{
					Name:           "ocr",
					TabletMode:     false,
					Query:          []string{"Thoughts"},
					ExpectedResult: "About",
					UseIca:         false,
					UseOcr:         true,
				},
			},
		},
	})
}

func SearchCategoriesImages(ctx context.Context, s *testing.State) {
	param := s.Param().(util.ImageSearchTestParam)

	cr := s.FixtValue().(fixture.LauncherSearchFixtData).Chrome
	tconn := s.FixtValue().(fixture.LauncherSearchFixtData).TestAPIConn
	kb := s.FixtValue().(fixture.LauncherSearchFixtData).Keyboard

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Gather required DLCs
	var dlcList []string
	if param.UseIca {
		dlcList = append(dlcList, "ml-core-internal")
	}
	if param.UseOcr {
		dlcList = append(dlcList, "screen-ai")
	}

	// TODO(b/303151432): Change the dlc force install to VerifyDlcInstalled when the bug is fixed.
	if err := launcher.InstallDlc(ctx, dlcList); err != nil {
		s.Fatal("Cannot install dlc: ", err)
	}

	if err := launcher.VerifyDlcInstalled(ctx, dlcList); err != nil {
		s.Fatal("Cannot find dlc: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	localFileLocation := filepath.Join(downloadsPath, localPictureName)
	if err := fsutil.CopyFile(s.DataPath(localPictureName), localFileLocation); err != nil {
		s.Fatalf("Failed to copy the test image to %s: %s", localFileLocation, err)
	}
	defer os.Remove(localFileLocation)

	ui := uiauto.New(tconn)

	for _, query := range param.Query {
		cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, param.TabletMode, false /*stabilizeAppCount*/)
		if err != nil {
			s.Fatal("Failed to set up launcher test case: ", err)
		}
		defer cleanup(cleanupCtx)

		if err := launcher.VerifyDlcInstalled(ctx, dlcList); err != nil {
			s.Fatal("Cannot verify dlc: ", err)
		}

		//GoBigSleepLint: Indexing may be slow on low-end devices.
		testing.Sleep(ctx, 5*time.Second)

		if err := uiauto.Retry(util.ImageSearchRetryTimes, uiauto.NamedCombine("Search for image",
			launcher.SearchWithCategory(tconn, kb, query, queryCategoryInfo),
			ui.Exists(util.ImageNode),
		))(ctx); err != nil {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump_"+s.Param().(util.ImageSearchTestParam).Name)
			s.Fatal("Failed to search image: ", err)
		}

		if err := launcher.ChangeSelectStatusOfSearchCategoryItem(tconn, ui, util.ImagesSelectionItem)(ctx); err != nil {
			s.Fatal("Failed to unselect Images in the menu : ", err)
		}

		if err := uiauto.Retry(util.ImageSearchRetryTimes, uiauto.NamedCombine("search image after unselect Images in menu",
			launcher.SearchWithCategory(tconn, kb, query, queryCategoryInfo),
			ui.Exists(util.ImageNode),
		))(ctx); err == nil {
			s.Fatal("The image is still present: ", err)
		}

		if err := launcher.ChangeSelectStatusOfSearchCategoryItem(tconn, ui, util.ImagesSelectionItem)(ctx); err != nil {
			s.Fatal("Failed to select Images in the menu : ", err)
		}

		if err := uiauto.Retry(util.ImageSearchRetryTimes, uiauto.NamedCombine("Search for image after select images in category",
			launcher.SearchWithCategory(tconn, kb, query, queryCategoryInfo),
			ui.Exists(util.ImageNode),
		))(ctx); err != nil {
			s.Fatal("Failed to search image after select images in category: ", err)
		}

	}

	if err := launcher.VerifyDlcInstalled(ctx, dlcList); err != nil {
		s.Fatal("Cannot find dlc: ", err)
	}
}
