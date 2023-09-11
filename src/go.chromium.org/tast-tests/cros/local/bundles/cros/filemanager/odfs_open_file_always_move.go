// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsOpenFileAlwaysMove,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that docx, xlsx and pptx open in OneDrive and we can see 'always move' checkbox for 2nd time",
		BugComponent: "b:1199143",
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"lucmult@chromium.org",
			"wenbojie@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		VarDeps: []string{
			"onedrive.accountPool",
		},
		Params: []testing.Param{{
			Fixture: "onedrive",
		}, {
			Name:              "lacros",
			Fixture:           "onedriveLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-68baf61c-8fdf-497d-8192-4da1e0ec0432",
		}, {
			Key:   "feature_id",
			Value: "screenplay-139eaefc-cd34-466f-8a2d-3a07a07a0009",
		}},
	})
}

// OdfsOpenFileAlwaysMove tests user opening the 3 file types:
// 1. docx: First file, goes through the setup flow.
// 2. pptx: Opens and  checks the "Don't ask again" in the move file confirmation dialog.
// 3. xlsx: Opens without the confirmation dialog.
func OdfsOpenFileAlwaysMove(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)

	for i, subTest := range data.GeneratedFiles {
		fileName := subTest.FileName
		fileType := subTest.FileType
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			files, err := filesapp.Launch(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to launch Files app: ", err)
			}
			defer files.Close(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_"+fileType)

			cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.OneDrive)
			if err != nil {
				s.Fatal("Failed to open office file: ", err)
			}
			ms365App, err := ms365.App(ctx, tconn, accountPool)
			if err != nil {
				s.Fatal("Failed to get instance of Ms365: ", err)
			}

			// The steps inside the IF are the initial setup that only happen in the first file.
			if i == 0 {
				options := &cloudupload.OneDriveSetupFlowOptions{
					Ms365App: ms365App, PWAInstalled: false, OneDriveConnected: false}
				if err := cloudUpload.RunOneDriveSetupFlow(options)(ctx); err != nil {
					s.Fatal("Failed to run the setup dialog steps: ", err)
				}
			}

			// Move/copy confirmation dialog.
			// 1st file: It doesn't show the "Don't ask again" option.
			// 2nd file: We want to check the "Don't ask again".
			// 3rd file: The dialog shouldn't show.
			if i < 2 {
				alwaysMove := false // 1st file.
				if i == 1 {
					alwaysMove = true // 2nd file.
				}

				if err := cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(alwaysMove)(ctx); err != nil {
					s.Fatal("Failed confirming to upload to cloud: ", err)
				}
			}

			if err := ms365App.WaitForMicrosoft365WindowAndClose(tconn, fileName)(ctx); err != nil {
				s.Fatal("Failed waiting file to open on MS365: ", fileName, err)
			}
			if err := onedrive.CheckODFSContent(ctx, subTest.SrcFile, fileName); err != nil {
				s.Fatal("ODFS upload didn't match: ", err)
			}
		}

		if !s.Run(ctx, fileType, f) {
			s.Errorf("Failed to run test in %q", fileType)
		}
	}
}
