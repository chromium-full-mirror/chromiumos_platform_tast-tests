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
		Func:         DrivefsOfficeOpenFile,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies docx, xlsx and pptx files can be open by Google Drive",
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
		Fixture: "onedriveAndGoogleDrive",
	})
}

// DrivefsOfficeOpenFile tests that opening a file from Downloads shows the file
// handler dialog, Google Drive can be selected, the file is moved to Drive and
// opened in the Editor
func DrivefsOfficeOpenFile(ctx context.Context, s *testing.State) {
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)
	driveFsClient := data.DriveFs

	for _, subTest := range data.GeneratedFiles {
		fileName := subTest.FileName
		fileType := subTest.FileType
		srcFile := subTest.SrcFile
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			files, err := filesapp.Launch(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to launch Files app: ", err)
			}
			defer files.Close(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "drive_open_office_file_"+fileType)
			defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

			cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.DriveFs)
			if err != nil {
				s.Fatal("Failed to open office file: ", err)
			}

			// Run Google Drive setup flow and confirm upload in Move/copy confirmation dialog.
			if err := uiauto.Combine("Confirm upload and wait to open",
				cloudUpload.RunGoogleDriveSetupFlow(),
				cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
				drivefs.WaitForGoogleDriveWindow(tconn, fileName),
			)(ctx); err != nil {
				s.Fatal("Failed to upload and open on Google Drive: ", fileName, err)
			}
			defer drivefs.CloseGoogleDriveWindow(cleanupCtx, tconn, fileName)

			if err := drivefs.VerifySourceDestinationMD5SumMatch(driveFsClient, srcFile, fileName)(ctx); err != nil {
				s.Fatal("Google Drive upload didn't match: ", err)
			}
		}

		if !s.Run(ctx, fileType, f) {
			s.Errorf("Failed to run test in %q", fileType)
		}
	}
}
