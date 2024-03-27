// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DrivefsOfficeOpenFromOdfs,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Verifies docx, xlsx and pptx files can be opened by Google Drive from ODFS",
		BugComponent:   "b:1199143",
		Timeout:        5 * time.Minute,
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
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-f3b76a6e-e081-4954-921c-65e0988177b0",
		}},
	})
}

// DrivefsOfficeOpenFromOdfs tests that opening a file from ODFS shows the file
// handler dialog, Google Drive can be selected, the file is copied to Drive and
// opened in the Editor. It tests this in three stages:
// 1. docx: First file, goes through the setup flow.
// 2. pptx: Opens and checks the "Don't ask again" in the copy file confirmation dialog.
// 3. xlsx: Opens without the confirmation dialog.
func DrivefsOfficeOpenFromOdfs(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	driveFsClient := data.DriveFs

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := office.ConnectToOneDrive(cr, tconn, filesApp, ms365App)(ctx); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	odfsToken, err := filesApp.GetOdfsFuseboxToken(ctx, data.Chrome)
	if err != nil {
		s.Fatal("Failed to get the ODFS key: ", err)
	}

	testFolderName := filemanager.GenerateTestFileName("_odfs_DrivefsOfficeOpenFromOdfs")
	// TODO(austinct): Ideally we would call fsutil.CopyDir to copy the test directory
	// into ODFS, but it fails because chown/chmod do not work on ODFS. Either implement
	// those calls for ODFS or modify CopyDir to work on filesystems without those calls.
	testFolderPath, err := filemanager.CreateFolderInFusebox(odfsToken, testFolderName)
	if err != nil {
		s.Fatalf("Failed to create test folder in ODFS %q: %s", testFolderName, err)
	}
	defer filemanager.RemovePathInFusebox(cleanupCtx, testFolderPath, 5*time.Second)
	for _, testFile := range data.GeneratedFiles {
		content, err := os.ReadFile(testFile.SrcFile)
		if err != nil {
			s.Fatalf("Failed to read test file %q: %s", testFile.SrcFile, err)
		}
		destination := filepath.Join(testFolderPath, testFile.FileName)
		if err := os.WriteFile(destination, content, 0755); err != nil {
			s.Fatalf("Failed to write test file %q: %s", destination, err)
		}
	}

	if err := filesApp.OpenPath(filesapp.FilesTitlePrefix+filesapp.OneDrive, filesapp.OneDrive, testFolderName)(ctx); err != nil {
		s.Fatalf("Failed to navigate to test folder inside ODFS %q: %s", testFolderName, err)
	}

	for i, testFile := range data.GeneratedFiles {
		cloudUpload, err := filesApp.OpenOfficeFile(ctx, "", testFile.FileName, filesconsts.DriveFs)
		if err != nil {
			s.Fatalf("Failed to open office file %q: %s", testFile.FileName, err)
		}

		var setupActions []uiauto.Action
		setupActions = append(setupActions, cloudUpload.RunGoogleDriveSetupFlow())
		if i < 2 {
			setupActions = append(setupActions, cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(i == 1 /*=alwaysMove*/))
		}
		setupActions = append(setupActions, drivefs.WaitForDSSWindowAndClose(tconn, testFile.FileName))

		if err := uiauto.Combine("Complete copy to Drive flow", setupActions...)(ctx); err != nil {
			s.Fatal("Failed to complete copy to Drive flow: ", err)
		}
		// Remove the copied office file from Drive by its cloud ID at the end of the test
		defer drivefs.RemoveDriveFsFileViaAPI(driveFsClient, data.DriveAPIClient, testFile.FileName)(cleanupCtx)

		if err := drivefs.VerifySourceDestinationMD5SumMatch(driveFsClient, testFile.SrcFile, testFile.FileName)(ctx); err != nil {
			s.Error("Google Drive upload didn't match: ", err)
		}

		if err := onedrive.CheckODFSContent(ctx, testFile.SrcFile, filepath.Join(testFolderName, testFile.FileName)); err != nil {
			s.Error("Failed to check that the office file still exists in ODFS: ", err)
		}
	}
}
