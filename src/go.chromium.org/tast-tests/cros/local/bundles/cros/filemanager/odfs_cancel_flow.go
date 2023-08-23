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
		Func:         OdfsCancelFlow,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that exiting Setup at various points restarts in the correct place",
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
	})
}

// OdfsCancelFlow tests that Setup flow can be cancelled at various points and
// restarts in the correct place.
func OdfsCancelFlow(ctx context.Context, s *testing.State) {
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_cancel_flow")

	cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.OneDrive)
	if err != nil {
		s.Fatal("Failed to open office file: ", err)
	}
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	// Open office file again after cancellation and choose provider.
	reopenOfficeFileAndChooseProvider := uiauto.Combine("Open office file after cancellation and choose provider",
		func(ctx context.Context) error {
			_, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.OneDrive)
			return err
		},
		cloudUpload.WaitFileHandlerAndChooseProvider(),
	)

	if err := uiauto.Combine("Setup OneDrive and open file with cancellation",
		cloudUpload.WaitFileHandlerAndChooseProvider(),
		cloudUpload.WaitGetStartedDialogAndClick(cloudupload.Cancel),
		reopenOfficeFileAndChooseProvider,
		cloudUpload.WaitGetStartedDialogAndClick(cloudupload.CancelThenNext),
		cloudUpload.WaitInstallPWADialogAndClick(cloudupload.Cancel),
		reopenOfficeFileAndChooseProvider,
		cloudUpload.WaitGetStartedDialogAndClick(cloudupload.Next),
		cloudUpload.WaitInstallPWADialogAndClick(cloudupload.CancelThenNext),
		cloudUpload.WaitConnectToOneDriveDialogAndClick(cloudupload.Cancel),
		reopenOfficeFileAndChooseProvider,
		cloudUpload.WaitGetStartedDialogAndClick(cloudupload.Next),
		cloudUpload.WaitConnectToOneDriveDialogAndClick(cloudupload.CancelThenNext),
		// Close auth window and it will go back to Connect OneDrive step with errors.
		ms365.CloseMicrosoftAuthWindowWithoutSignIn(tconn),
		cloudUpload.WaitConnectOneDriveError(),
		// Connect to OneDrive again.
		cloudUpload.WaitConnectToOneDriveDialogAndClick(cloudupload.Next),
		ms365App.LoginToMicrosoft365(cloudupload.SetupCompleteDialog, false /*=skipPassword*/),
		cloudUpload.WaitSetupCompleteDialogAndClickDone(),
		// Move/copy confirmation dialog.
		cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
		ms365App.WaitForMicrosoft365WindowAndCloseIgnoreError(tconn, fileName),
	)(ctx); err != nil {
		s.Fatal("Failed to setup OneDrive with cancellation: ", fileName, err)
	}

	if err := onedrive.CheckODFSContent(ctx, srcFile, fileName); err != nil {
		s.Fatal("ODFS upload didn't match: ", err)
	}
}
