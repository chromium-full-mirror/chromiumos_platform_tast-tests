// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DrivefsOfficeAskBeforeMoving,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that toggling the 'Ask Before Moving to Google Drive' option makes the move interstitial disappear for Setup flow",
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
		Fixture: "onedriveAndGoogleDrive",
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-40938f8a-adf8-4c3d-a0c0-cc12c09df410",
		}},
	})
}

// DrivefsOfficeAskBeforeMoving tests that toggling the "Ask Before Moving to Google Drive"
// option in Settings makes the move confirmation disappear for Setup flow.
// It will open 2 office files:
// 1. Open the first file with full setup flow.
// 2. Go to Settings and disable "Ask before moving".
// 3. Open the second file, the flow should skip Move confirmation dialog.
func DrivefsOfficeAskBeforeMoving(ctx context.Context, s *testing.State) {
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)
	driveFsClient := data.DriveFs

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_ask_before_moving")
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	// Pick one file from the generated files.
	fileName1 := data.GeneratedFiles[0].FileName
	srcFile1 := data.GeneratedFiles[0].SrcFile

	cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName1, filesconsts.DriveFs)
	if err != nil {
		s.Fatal("Failed to open the first office file: ", err)
	}

	// Open first file with the full setup flow.
	if err := uiauto.Combine("Run Google Drive setup flow",
		cloudUpload.RunGoogleDriveSetupFlow(),
		cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
		drivefs.WaitForDSSWindowAndClose(tconn, fileName1),
	)(ctx); err != nil {
		s.Fatal("Failed to upload and open the first file on MS365: ", fileName1, err)
	}
	if err := drivefs.VerifySourceDestinationMD5SumMatch(driveFsClient, srcFile1, fileName1)(ctx); err != nil {
		s.Fatal("Google Drive upload for the first file didn't match: ", err)
	}

	// Disable "Ask before moving" for Google Drive in Settings.
	settingsApp := ossettings.New(tconn)
	toggleOptionName := "Ask before copying or moving Microsoft files to Google Drive"
	if err := uiauto.Combine("Disable 'Ask before moving' for Google Drive in Settings",
		office.LaunchOfficeFilesSettingsPage(cr, tconn),
		settingsApp.WaitUntilToggleOption(cr, toggleOptionName, true /*=expected*/),
		settingsApp.SetToggleOption(cr, toggleOptionName, false /*=expected*/),
		settingsApp.Close,
	)(ctx); err != nil {
		s.Fatal("Failed to disable 'Ask before moving' for Google Drive in Settings: ", err)
	}

	// Open another file.
	fileName2 := data.GeneratedFiles[1].FileName
	srcFile2 := data.GeneratedFiles[1].SrcFile

	if _, err := files.OpenOfficeFile(ctx, "", fileName2, filesconsts.DriveFs); err != nil {
		s.Fatal("Failed to open the second office file: ", err)
	}
	if err := uiauto.Combine("Upload and Open the second file",
		cloudUpload.WaitFileHandlerAndChooseProvider(),
		drivefs.WaitForDSSWindowAndClose(tconn, fileName2),
	)(ctx); err != nil {
		s.Fatal("Failed to upload and open the second file on Google Drive: ", fileName2, err)
	}
	if err := drivefs.VerifySourceDestinationMD5SumMatch(driveFsClient, srcFile2, fileName2)(ctx); err != nil {
		s.Fatal("Google Drive upload for the second file didn't match: ", err)
	}
}
