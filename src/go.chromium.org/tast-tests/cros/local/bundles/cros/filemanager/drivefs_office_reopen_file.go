// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DrivefsOfficeReopenFile,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Verifies that office file can be open directly in Google Drive",
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
		Params: []testing.Param{{
			Fixture: "onedriveAndGoogleDrive",
		}, {
			Name:              "lacros",
			Fixture:           "onedriveAndGoogleDriveLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-a71f5740-a8f7-455b-89b6-4359a98a96cf",
		}},
	})
}

// DrivefsOfficeReopenFile tests user opening the 1 docx file, going through the setup flow.
// Then opening the same file directly from the Google Drive, which should open without any setup, copy or move.
func DrivefsOfficeReopenFile(ctx context.Context, s *testing.State) {
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)
	driveFsClient := data.DriveFs

	subTest := data.GeneratedFiles[0]
	fileName := subTest.FileName
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "drivefs_reopen_office_file")
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	// Opening the docx file from Downloads.
	cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.DriveFs)
	if err != nil {
		s.Fatal("Failed to open office file: ", err)
	}
	// Run Google Drive setup flow and confirm upload in Move/copy confirmation dialog.
	if err := uiauto.Combine("Confirm upload and wait to open",
		cloudUpload.RunGoogleDriveSetupFlow(),
		cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
		drivefs.WaitForDSSWindowAndClose(tconn, fileName),
	)(ctx); err != nil {
		s.Fatal("Failed to upload and open in Google Drive: ", fileName, err)
	}

	// Open the file again from Google Drive.
	if _, err := files.OpenOfficeFile(ctx, filesapp.GoogleDrive, fileName, filesconsts.DriveFs); err != nil {
		s.Fatal("Failed to open file from Google Drive: ", err)
	}
	if err := drivefs.WaitForDSSWindowAndCloseIgnoreError(tconn, fileName)(ctx); err != nil {
		s.Fatal("Failed to upload and open in Google Drive: ", fileName, err)
	}

	if err := drivefs.VerifySourceDestinationMD5SumMatch(driveFsClient, subTest.SrcFile, fileName)(ctx); err != nil {
		s.Fatal("Google Drive upload didn't match: ", err)
	}
}
