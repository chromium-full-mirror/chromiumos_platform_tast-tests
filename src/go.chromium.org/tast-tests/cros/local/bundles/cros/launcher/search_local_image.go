// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/launcher/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const localPictureName = "search_local_image.png"

type testParam struct {
	TabletMode     bool
	Query          string
	ExpectedResult string
	UseIca         bool
	UseOcr         bool
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
		Attr:         []string{"group:launcher_image_search_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{localPictureName},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name: "search_by_paper_lowercase_lca",
				Val: testParam{
					TabletMode:     false,
					Query:          "paper",
					ExpectedResult: "Thoughts",
					UseIca:         true,
					UseOcr:         false,
				},
				Fixture: fixture.LauncherImageSearchIca,
			},
			{
				Name: "search_by_paper_uppercase_lca",
				Val: testParam{
					TabletMode:     false,
					Query:          "Paper",
					ExpectedResult: "Thoughts",
					UseIca:         true,
					UseOcr:         false,
				},
				Fixture: fixture.LauncherImageSearchIca,
			},
			{
				Name: "search_by_content_ocr",
				Val: testParam{
					TabletMode:     false,
					Query:          "Thoughts",
					ExpectedResult: "About",
					UseIca:         false,
					UseOcr:         true,
				},
				Fixture: fixture.LauncherImageSearchOcr,
			},
			{
				Name: "search_by_paper_ica_ocr",
				Val: testParam{
					TabletMode:     false,
					Query:          "Paper",
					ExpectedResult: "Thoughts",
					UseIca:         true,
					UseOcr:         true,
				},
				Fixture: fixture.LauncherImageSearchIcaAndOcr,
			},
			{
				Name: "search_by_content_ica_ocr",
				Val: testParam{
					TabletMode:     false,
					Query:          "Thoughts",
					ExpectedResult: "About",
					UseIca:         true,
					UseOcr:         true,
				},
				Fixture: fixture.LauncherImageSearchIcaAndOcr,
			},
		},
	})
}

func SearchLocalImage(ctx context.Context, s *testing.State) {
	param := s.Param().(testParam)

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

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	localFileLocation := filepath.Join(downloadsPath, localPictureName)
	if err := fsutil.CopyFile(s.DataPath(localPictureName), localFileLocation); err != nil {
		s.Fatalf("Failed to copy the test image to %s: %s", localFileLocation, err)
	}
	defer os.Remove(localFileLocation)

	tabletMode := param.TabletMode
	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	ui := uiauto.New(tconn)
	ud := uidetection.NewDefault(tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	query := param.Query
	expectedResult := param.ExpectedResult
	picture := nodewith.Role(role.ListBoxOption).HasClass("SearchResultImageView").NameContaining("search_local_image")

	if err := uiauto.Retry(2, uiauto.NamedCombine("Search for image",
		launcher.ClearSearchField(tconn, kb),
		launcher.Search(tconn, kb, query),
		launcher.WaitForResultWithCategory(tconn, launcher.SearchCategoryInfo{
			Category:  "Images",
			NeedRegex: false,
			Result:    "search_local_image",
		}),
		ui.DoDefault(picture),
		launcher.VerifyTextWithUIDetection(ud, expectedResult),
	))(ctx); err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Failed to search image: ", err)

	}
}
