// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsFileCrud,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies basic file operations (create/rename/delete) work in ODFS",
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

// OdfsFileCrud tests the basic file operations like create/rename/delete work in ODFS.
func OdfsFileCrud(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_file_crud")

	// Connect to OneDrive.
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := files.ConnectToOneDrive(ctx, ms365App); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	// Create a file in ODFS via fusebox (no UI to create file in ODFS).
	odfsToken, err := files.GetOdfsFuseboxToken(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get ODFS key: ", err)
	}
	fileName := "odfs_test.txt"
	uniqueFileName := filemanager.GenerateTestFileName(fileName)
	fileContent := "test"
	fullPath, err := filemanager.CreateFileInFusebox(odfsToken, uniqueFileName, fileContent)
	if err != nil {
		s.Fatal("Failed to create file in ODFS: ", err)
	}

	// Check file existence and file content.
	if err := uiauto.Combine("Check the file is available in Files app",
		files.OpenOneDrive(),
		files.WaitForFile(uniqueFileName),
	)(ctx); err != nil {
		s.Fatal("Failed to find the newly created file in Files app: ", err)
	}

	// It takes around 20s for the content to be shown in Files, give it a bit buffer.
	timeout := 40 * time.Second
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		contentGot, err := os.ReadFile(fullPath)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get content of the newly created file"))
		}
		if fileContent == string(contentGot) {
			return nil
		}
		return errors.Errorf("the file content doesn't match, want: %q, got: %q", fileContent, string(contentGot))
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		s.Fatal("Failed to check the file content: ", err)
	}

	// Rename file.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}
	renamedFileName := filemanager.GenerateTestFileName(fileName)
	if err := uiauto.Combine("Rename the file",
		files.RenameFile(kb, uniqueFileName, renamedFileName),
		files.WaitUntilFileGone(uniqueFileName),
	)(ctx); err != nil {
		s.Fatal("Failed to rename the file: ", err)
	}

	// Delete file.
	if err := files.DeleteFileOrFolder(kb, renamedFileName)(ctx); err != nil {
		s.Fatal("Failed to delete the file: ", err)
	}
}
