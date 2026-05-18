// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const localImageName = "dog_with_words.jpg"

func init() {
	testing.AddTest(&testing.Test{
		Func: SearchLocalImage,
		Desc: "Checks that use local image search with different feature flags and search for a local image",
		Contacts: []string{
			"launcher-search-notify@google.com",
			"chenjih@google.com",
		},
		BugComponent: "b:1281467",
		SoftwareDeps: []string{"chrome"},
		Data:         []string{localImageName},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name: "search_with_ica",
				Val: util.ImageSearchTestParam{
					Name:           "ica",
					TabletMode:     false,
					Query:          []string{"dog", "Dog"},
					ExpectedResult: "About",
					UseIca:         true,
					UseOcr:         false,
				},
				Fixture:           fixture.LauncherImageSearchIca,
				ExtraSoftwareDeps: []string{"ondevice_image_content_annotation"},
				ExtraAttr: []string{"group:cbx",
					"cbx_feature_enabled",
					"cbx_stable"},
				ExtraTestBedDeps: []string{tbdep.Cbx(true)},
			},
			{
				Name: "search_with_ocr_critical",
				Val: util.ImageSearchTestParam{
					Name:           "ocr",
					TabletMode:     false,
					Query:          []string{"Pies"},
					ExpectedResult: "About",
					UseIca:         false,
					UseOcr:         true,
				},
				Fixture:           fixture.LauncherImageSearchOcr,
				ExtraSoftwareDeps: []string{"ondevice_image_content_annotation"},
				ExtraAttr: []string{"group:cbx",
					"cbx_feature_enabled",
					"cbx_stable"},
				ExtraTestBedDeps: []string{tbdep.Cbx(true)},
			},
			{
				Name: "search_with_ica_ocr",
				Val: util.ImageSearchTestParam{
					Name:           "ica_ocr",
					TabletMode:     false,
					Query:          []string{"Dog", "Pies"},
					ExpectedResult: "About",
					UseIca:         true,
					UseOcr:         true,
				},
				Fixture:           fixture.LauncherImageSearchIcaAndOcr,
				ExtraSoftwareDeps: []string{"ondevice_image_content_annotation"},
				ExtraAttr: []string{"group:cbx",
					"cbx_feature_enabled",
					"cbx_stable"},
				ExtraTestBedDeps: []string{tbdep.Cbx(true)},
			},
		},
	})
}

func SearchLocalImage(ctx context.Context, s *testing.State) {
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
		dlcList = append(dlcList, "ml-core-dlc")
	}
	if param.UseOcr {
		dlcList = append(dlcList, "screen-ai")
	}

	if err := launcher.VerifyDlcInstalled(ctx, dlcList); err != nil {
		s.Fatal("Cannot find dlc: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	localFileLocation := filepath.Join(downloadsPath, localImageName)
	if err := fsutil.CopyFile(s.DataPath(localImageName), localFileLocation); err != nil {
		s.Fatalf("Failed to copy the test image to %s: %s", localFileLocation, err)
	}
	defer os.Remove(localFileLocation)

	ui := uiauto.New(tconn)
	imageNode := nodewith.Role(role.StaticText).ClassName("Label").Name(localImageName)
	homeButtonFinder := nodewith.Name("Launcher").Role(role.Button).Ancestor(nodewith.HasClass("ShelfContainer"))

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
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump_"+s.Param().(util.ImageSearchTestParam).Name)

		if err := uiauto.Retry(util.ImageSearchRetryTimes, uiauto.NamedCombine("Search for image",
			launcher.SearchWithCategory(tconn, kb, query, launcher.SearchCategoryInfo{
				Category:  "Images",
				NeedRegex: false,
				Result:    "search_local_image",
			}),
			ui.Exists(imageNode),
			ui.DoDefault(homeButtonFinder),
		))(ctx); err != nil {
			s.Fatal("Failed to search image: ", err)
		}
	}

	if err := launcher.VerifyDlcInstalled(ctx, dlcList); err != nil {
		s.Fatal("Cannot find dlc: ", err)
	}
}
