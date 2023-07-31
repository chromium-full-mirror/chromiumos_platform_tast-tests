// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
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
		Func:         OdfsReopenFile,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that docx, xlsx and pptx open in OneDrive",
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
		Fixture: "onedrive",
	})
}

// OdfsReopenFile tests user opening the 1 docx file, going through the setup flow.
// Then opening the same file directly from the ODFS, which should open without any setup, copy or move.
func OdfsReopenFile(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	targetBaseName := filepath.Base(data.TargetFolder)

	subTest := data.GeneratedFiles[0]
	fileName := subTest.FileName
	fileType := subTest.FileType
	f := func(ctx context.Context, s *testing.State) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		files, err := filesapp.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch Files app: ", err)
		}
		defer files.Close(cleanupCtx)
		// Close the Office 365 to avoid interfere with following tests and allow the file deletion in the fixture.
		defer ash.CloseAllWindows(cleanupCtx, tconn)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_"+fileType)

		// Opening the docx file from Downloads.
		cloudUpload, err := files.OpenOfficeFile(ctx, targetBaseName, fileName, onedrive.OneDrive)
		if err != nil {
			s.Fatal("Failed to open office file: ", err)
		}
		ms365App, err := ms365.App(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get instance of Ms365: ", err)
		}
		if err := cloudupload.RunOneDriveSetupFlow(ctx, accountPool, cloudUpload, ms365App); err != nil {
			s.Fatal("Failed to run the setup dialog steps: ", err)
		}

		// Move/copy confirmation dialog.
		if err := cloudUpload.MaybeConfirmUploadOr365Window(ms365App, fileName)(ctx); err != nil {
			s.Fatalf("Failed to upload and open on MS365: %q - %v", fileName, err)
		}

		// Close the MS365 window.
		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			if strings.Contains(w.Title, fileName) && strings.Contains(w.Title, "Microsoft") {
				return true
			}
			return false
		})
		if err != nil {
			s.Fatal("Failed to find the MS365 window to close")
		}
		if err := w.CloseWindow(ctx, tconn); err != nil {
			s.Fatal("Failed to close the MS365 window")
		}

		// Open the file again from OneDrive.
		if _, err := files.OpenOfficeFile(ctx, filesapp.OneDrive, fileName, onedrive.OneDrive); err != nil {
			s.Fatal("Failed to open OneDrive: ", err)
		}
		if err := ms365App.WaitForMicrosoft365Window(fileName)(ctx); err != nil {
			s.Fatal("Failed to upload and open on MS365: ", fileName, err)
		}

		if err := onedrive.CheckODFSContent(ctx, subTest.SrcFile, fileName); err != nil {
			s.Fatal("ODFS upload didn't match: ", err)
		}
	}

	if !s.Run(ctx, fileType, f) {
		s.Errorf("Failed to run test in %q", fileType)
	}
}
