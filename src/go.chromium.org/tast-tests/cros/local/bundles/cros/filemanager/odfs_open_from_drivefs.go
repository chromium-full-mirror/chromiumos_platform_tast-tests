// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsOpenFromDrivefs,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies docx, xlsx and pptx files can be opened from DriveFS with Microsoft 365",
		BugComponent: "b:1199143",
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"austinct@chromium.org",
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
			Fixture: "onedriveAndGoogleDrive",
		}, {
			Name:              "lacros",
			Fixture:           "onedriveAndGoogleDriveLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// OdfsOpenFromDrivefs tests users opening office files from DriveFS with Microsoft 365.
// It tests three types of office files:
// 1. docx: First file, goes through the setup flow.
// 2. pptx: Opens and checks the "Don't ask again" in the copy file confirmation dialog.
// 3. xlsx: Opens without the confirmation dialog.
func OdfsOpenFromDrivefs(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	tconn := data.TestAPIConn
	testDataFolder := data.TargetFolder
	driveFsClient := data.DriveFs
	driveAPIClient := data.DriveAPIClient

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	uniqueDriveFolderName := fmt.Sprintf("OdfsOpenFromDrivefs-%s", uuid.New())
	driveFolder, err := driveAPIClient.CreateFolder(ctx, uniqueDriveFolderName, []string{"root"})
	if err != nil {
		s.Fatal("Failed to create test folder in My Drive: ", err)
	}
	defer driveAPIClient.RemoveFileByID(cleanupCtx, driveFolder.Id)

	filesApp, err := filesapp.LaunchSWAToPath(ctx, tconn, driveFsClient.MyDrivePath(uniqueDriveFolderName))
	if err != nil {
		s.Fatal("Failed to launch files app to the test folder in My Drive: ", err)
	}

	for _, testFile := range data.GeneratedFiles {
		if err := fsutil.CopyFile(filepath.Join(testDataFolder, testFile.FileName), driveFsClient.MyDrivePath(uniqueDriveFolderName, testFile.FileName)); err != nil {
			s.Fatalf("Failed to copy test file %q to My Drive: %s", testFile.FileName, err)
		}
	}

	// Wait for one of the files to appear. This may take longer than the usual
	// 15 second timeout, so do this separately with a bigger timeout.
	if err := filesApp.WithTimeout(30 * time.Second).WaitForFile(data.GeneratedFiles[0].FileName)(ctx); err != nil {
		s.Fatalf("Failed to wait for office file %q to appear: %s", data.GeneratedFiles[0].FileName, err)
	}

	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	for i, testFile := range data.GeneratedFiles {
		cloudUpload, err := filesApp.OpenOfficeFile(ctx, "", testFile.FileName, filesconsts.OneDrive)
		if err != nil {
			s.Fatalf("Failed to open office file %q: %s", testFile.FileName, err)
		}

		var setupActions []uiauto.Action
		if i == 0 {
			setupActions = append(setupActions, cloudUpload.RunOneDriveSetupFlow(&cloudupload.OneDriveSetupFlowOptions{
				Ms365App: ms365App, PWAInstalled: false, OneDriveConnected: false}))
		}
		if i <= 1 {
			// For the second file we enable always move to OneDrive, hence we
			// don't need to wait for any confirmation dialogs in subsequent files
			setupActions = append(setupActions, cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(i == 1 /*alwaysMove*/))
		}
		setupActions = append(setupActions, ms365App.WaitForMicrosoft365WindowAndClose(tconn, testFile.FileName))

		if err := uiauto.Combine("complete copy to One Drive flow", setupActions...)(ctx); err != nil {
			s.Fatal("Failed to complete copy to One Drive flow: ", err)
		}

		if err := onedrive.CheckODFSContent(ctx, testFile.SrcFile, testFile.FileName); err != nil {
			s.Fatal("ODFS upload didn't match: ", err)
		}

		// Ensure the file is copied rather than moved.
		if err := uiauto.Combine("check the file still exists in Drive",
			filesApp.FileExists(testFile.FileName),
			drivefs.VerifySourceDestinationMD5SumMatch(
				driveFsClient, testFile.SrcFile, filepath.Join(uniqueDriveFolderName, testFile.FileName)),
		)(ctx); err != nil {
			s.Fatal("Failed to check the file still exists in Drive: ", err)
		}
	}
}
