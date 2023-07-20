// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsOpenFile,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that docx, xlsx and pptx open in OneDrive",
		BugComponent: "b:1199143",
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"lucmult@chromium.org",
			"alexbn@chromium.org",
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

func OdfsOpenFile(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)

	for i, subTest := range []struct {
		fType   string
		fName   string
		srcFile string
	}{
		{"docx", data.DocxName, data.SrcDocx},
		{"pptx", data.PptxName, data.SrcPptx},
		{"xlsx", data.XlsxName, data.SrcXlsx},
	} {
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			dumpName := "ui_" + subTest.fType
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, dumpName)

			fName := subTest.fName

			files, err := filesapp.Launch(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to launch Files app: ", err)
			}
			defer files.Close(cleanupCtx)
			// Close the Office 365 to avoid interfere with following tests and allow the file deletion in the fixture.
			defer ash.CloseAllWindows(cleanupCtx, tconn)

			cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fName, onedrive.OneDrive)
			if err != nil {
				s.Fatal("Failed to open office file: ", err)
			}
			ms365App, err := ms365.App(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get instance of Ms365: ", err)
			}

			// The steps inside the IF are the initial setup that only happen in the first file.
			if i == 0 {
				if err := cloudupload.RunOneDriveSetupFlow(ctx, accountPool, cloudUpload, ms365App); err != nil {
					s.Fatal("Failed to run the setup dialog steps: ", err)
				}
			}

			// Move/copy confirmation dialog.
			if err := uiauto.Combine("Confirm upload and wait to open",
				cloudUpload.WaitUploadConfirmationDialogAndClickToUpload(),
				ms365App.WaitForMicrosoft365Window(fName),
			)(ctx); err != nil {
				s.Fatal("Failed to upload and open on MS365: ", fName, err)
			}

			if err := onedrive.CheckODFSContent(ctx, subTest.srcFile, fName); err != nil {
				s.Fatal("ODFS upload didn't match: ", err)
			}
		}

		if !s.Run(ctx, subTest.fType, f) {
			s.Errorf("Failed to run test in %q", subTest.fType)
		}
	}
}
