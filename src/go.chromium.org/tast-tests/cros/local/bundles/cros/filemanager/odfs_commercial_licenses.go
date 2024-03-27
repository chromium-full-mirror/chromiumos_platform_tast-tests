// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
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

type userParam struct {
	username       string
	password       string
	useAccountPool bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           OdfsCommercialLicenses,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that opening a file with OneDrive works on managed users with various Microsoft licenses",
		BugComponent:   "b:1401215", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP > Enterprise Clippy
		Timeout:        5 * time.Minute,
		Contacts: []string{
			"cros-commercial-clippy-eng@google.com",
			"lmasopust@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier",
		},
		VarDeps: []string{
			"onedrive.accountPool",
			"onedrive.managedusernamemicrosoft",
			"onedrive.managedpassword",
		},
		Fixture: "onedriveManaged",
		Params: []testing.Param{{
			Name: "consumer",
			Val: userParam{
				useAccountPool: true,
			},
		}, {
			Name: "m365_business_standard",
			Val: userParam{
				useAccountPool: false,
				username:       "onedrive.managedusernamemicrosoft",
				password:       "onedrive.managedpassword",
			},
			// This test should cover more Microsoft licenses (e3, e5) but the
			// test setup is not ready yet, so they will be added later on.
		}},
	})
}

// OdfsCommercialLicenses tests user opening an office file with different
// Microsoft account types (consumer account, M365 business standard license, E3
// license, E5 license)
func OdfsCommercialLicenses(ctx context.Context, s *testing.State) {
	var creds credconfig.Creds

	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)

	param := s.Param().(userParam)
	if param.useAccountPool {
		msCreds, err := credconfig.PickRandomCreds(accountPool)
		if err != nil {
			s.Fatal("Failed to get the user/passwd for Microsoft 365: ", err)
		}
		creds = msCreds
	} else {
		creds.User = s.RequiredVar(param.username)
		creds.Pass = s.RequiredVar(param.password)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if len(data.GeneratedFiles) < 1 {
		s.Fatal("Failed get office files")
	}

	testFile := data.GeneratedFiles[0]
	fileName := testFile.FileName
	fileType := testFile.FileType

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_"+fileType)

	cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName,
		filesconsts.OneDrive)
	if err != nil {
		s.Fatal("Failed to open office file: ", err)
	}
	ms365App, err := ms365.AppWithCreds(ctx, tconn, creds)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	options := &cloudupload.OneDriveSetupFlowOptions{
		Ms365App: ms365App, PWAInstalled: false, OneDriveConnected: false}
	if err := cloudUpload.RunOneDriveSetupFlow(options)(ctx); err != nil {
		s.Fatal("Failed to run the setup dialog steps: ", err)
	}

	// Move/copy confirmation dialog.
	if err := uiauto.Combine("Confirm upload and wait to open",
		cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(false /*=alwaysMove*/),
		ms365App.WaitForMicrosoft365WindowAndClose(tconn, fileName),
	)(ctx); err != nil {
		s.Fatalf("Failed to upload and open on MS365: %q: %v", fileName, err)
	}

	if err := onedrive.CheckODFSContent(ctx, testFile.SrcFile, fileName); err != nil {
		s.Fatal("ODFS upload didn't match: ", err)
	}

}
