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
		Func:         OdfsWithOneDriveConnected,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies OneDrive can be connected separately before opening office files",
		BugComponent: "b:288018298",
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
		Fixture: "onedrive",
	})
}

// OdfsWithOneDriveConnected tests that Setup flow can recognize that ODFS is installed and just installs MS365 and begins the move process with a file from Downloads
func OdfsWithOneDriveConnected(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)

	// Pick one file from the generated files.
	fileName := data.GeneratedFiles[0].FileName
	srcFile := data.GeneratedFiles[0].SrcFile

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_with_one_drive_installed")

	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	// Connect to OneDrive via Files gear menu.
	cloudUpload := cloudupload.App(tconn, filesconsts.OneDrive)
	if err := uiauto.Combine("Connect to OneDrive via Files context menu",
		files.ClickMoreMenuItem("Services", "Connect OneDrive"),
		cloudUpload.WaitConnectToOneDriveDialogAndClickConnect(),
		ms365App.LoginToMicrosoft365(cloudupload.OneDriveConnectedDialog, false /*=skipPassword*/),
		cloudUpload.WaitOneDriveConnectedDialogAndClickClose(),
	)(ctx); err != nil {
		s.Fatal("Failed to click Connect OneDrive: ", err)
	}

	_, err = files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.OneDrive)
	if err != nil {
		s.Fatal("Failed to open office file: ", err)
	}

	options := &cloudupload.OneDriveSetupFlowOptions{
		Ms365App: ms365App, PWAInstalled: false, OneDriveConnected: true}
	if err := cloudUpload.RunOneDriveSetupFlow(options)(ctx); err != nil {
		s.Fatal("Failed to run the setup dialog steps: ", err)
	}

	// Move/copy confirmation dialog.
	if err := uiauto.Combine("Confirm upload and wait to open",
		cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
		ms365App.WaitForMicrosoft365Window(fileName),
	)(ctx); err != nil {
		s.Fatal("Failed to upload and open on MS365: ", fileName, err)
	}
	defer ms365.CloseMicrosoft365Window(cleanupCtx, tconn, fileName)

	if err := onedrive.CheckODFSContent(ctx, srcFile, fileName); err != nil {
		s.Fatal("ODFS upload didn't match: ", err)
	}
}
