// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
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
		Func:           OdfsOpenFile,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Verifies that docx, xlsx and pptx open in OneDrive",
		BugComponent:   "b:1199143",
		Timeout:        5 * time.Minute,
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
			Value: "screenplay-9dc29d03-fea0-498d-b1b3-e32f72d6cb73",
		}},
	})
}

// OdfsOpenFile tests user opening the 3 file types:
// 1. docx: First file, goes through the setup flow.
// 2. pptx: Opens and goes through the "move file" confirmation dialog.
// 3. xlsx: Same as pptx.
func OdfsOpenFile(ctx context.Context, s *testing.State) {
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
			if err := uiauto.Combine("Confirm upload and wait to open",
				cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
				ms365App.WaitForMicrosoft365WindowAndClose(tconn, fileName),
			)(ctx); err != nil {
				s.Fatalf("Failed to upload and open on MS365: %q: %v", fileName, err)
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
