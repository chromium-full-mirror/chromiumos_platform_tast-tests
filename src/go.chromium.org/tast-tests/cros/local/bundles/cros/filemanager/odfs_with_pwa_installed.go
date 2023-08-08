// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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
		Func:         OdfsWithPWAInstalled,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verifies office PWA can be installed separately before opening office files",
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
			Val:     browser.TypeAsh,
		},
		// TODO(b/293795743): enable Lacros variant
		// {
		// 	Name:              "lacros",
		// 	Fixture:           "onedriveLacros",
		// 	ExtraSoftwareDeps: []string{"lacros"},
		// 	Val:               browser.TypeLacros,
		// },
		},
	})
}

// OdfsWithPWAInstalled tests that Setup flow can recognize that MS365 is installed and just installs OneDrive and begins the move process with a file from Downloads.
func OdfsWithPWAInstalled(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	bt := s.Param().(browser.Type)
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)

	// Install Office PWA.
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := ms365App.InstallPWA(ctx, cr, bt); err != nil {
		s.Fatal("Failed to install Office PWA: ", err)
	}
	// Office PWA window will open after installation, we need to close it.
	// Here we just pass an empty file name string to close the PWA app itself.
	if err := ms365.CloseMicrosoft365Window(ctx, tconn, ""); err != nil {
		s.Fatal("Failed to close the Office PWA window: ", err)
	}

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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_with_pwa_installed")

	cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, filesconsts.OneDrive)
	if err != nil {
		s.Fatal("Failed to open office file: ", err)
	}

	options := &cloudupload.OneDriveSetupFlowOptions{
		Ms365App: ms365App, PWAInstalled: true, OneDriveConnected: false}
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
