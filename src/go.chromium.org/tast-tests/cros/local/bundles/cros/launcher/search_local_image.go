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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
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
	Name           string
	TabletMode     bool
	Query          []string
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
			"launcher-search-notify@google.com",
			"dgrebenyuk@google.com",
			"ypitsishin@google.com",
		},
		BugComponent: "b:1281467",
		SoftwareDeps: []string{"chrome"},
		Data:         []string{localPictureName},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name: "search_with_ica",
				Val: testParam{
					Name:           "ica",
					TabletMode:     false,
					Query:          []string{"paper", "Paper"},
					ExpectedResult: "About",
					UseIca:         true,
					UseOcr:         false,
				},
				Fixture:           fixture.LauncherImageSearchIca,
				ExtraSoftwareDeps: []string{"ondevice_image_content_annotation"},
				ExtraAttr: []string{"group:cbx",
					"cbx_feature_enabled",
					"cbx_unstable"},
			},
			{
				Name: "search_with_ocr_critical",
				Val: testParam{
					Name:           "ocr",
					TabletMode:     false,
					Query:          []string{"Thoughts"},
					ExpectedResult: "About",
					UseIca:         false,
					UseOcr:         true,
				},
				Fixture:           fixture.LauncherImageSearchOcr,
				ExtraSoftwareDeps: []string{"ondevice_image_content_annotation"},
				ExtraAttr: []string{"group:cbx",
					"cbx_feature_enabled",
					"cbx_unstable"},
			},
			{
				Name: "search_with_ocr_informational",
				Val: testParam{
					Name:           "ocr",
					TabletMode:     false,
					Query:          []string{"Thoughts"},
					ExpectedResult: "About",
					UseIca:         false,
					UseOcr:         true,
				},
				Fixture:           fixture.LauncherImageSearchOcr,
				ExtraSoftwareDeps: []string{"no_ondevice_image_content_annotation"},
				ExtraAttr:         []string{"group:mainline", "informational"},
			},
			{
				Name: "search_with_ica_ocr",
				Val: testParam{
					Name:           "ica_ocr",
					TabletMode:     false,
					Query:          []string{"Paper", "Thoughts"},
					ExpectedResult: "About",
					UseIca:         true,
					UseOcr:         true,
				},
				Fixture:           fixture.LauncherImageSearchIcaAndOcr,
				ExtraSoftwareDeps: []string{"ondevice_image_content_annotation"},
				ExtraAttr: []string{"group:cbx",
					"cbx_feature_enabled",
					"cbx_unstable"},
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

		ui := uiauto.New(tconn)
		ud := uidetection.NewDefault(tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
		picture := nodewith.Role(role.ListBoxOption).HasClass("SearchResultImageView").NameContaining("search_local_image")

		if err := uiauto.Retry(20, uiauto.NamedCombine("Search for image",
			launcher.ClearSearchField(tconn, kb),
			launcher.Search(tconn, kb, query),
			launcher.WaitForResultWithCategory(tconn, launcher.SearchCategoryInfo{
				Category:  "Images",
				NeedRegex: false,
				Result:    "search_local_image",
			}),
			ui.DoDefault(picture),
			launcher.VerifyTextWithUIDetection(ud, param.ExpectedResult),
		))(ctx); err != nil {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump_"+s.Param().(testParam).Name)
			s.Fatal("Failed to search image: ", err)
		}
	}

	if err := launcher.VerifyDlcInstalled(ctx, dlcList); err != nil {
		s.Fatal("Cannot find dlc: ", err)
	}
}
