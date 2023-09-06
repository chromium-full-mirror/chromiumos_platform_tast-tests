// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsFolderCrud,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies basic folder operations (create/rename/delete) work in ODFS",
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

// OdfsFolderCrud tests the basic folder operations like create/rename/delete work in ODFS.
func OdfsFolderCrud(ctx context.Context, s *testing.State) {
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_folder_crud")

	// Connect to OneDrive.
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := files.ConnectToOneDrive(ctx, ms365App); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	odfsToken, err := files.GetOdfsFuseboxToken(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get ODFS key: ", err)
	}

	// Create a folder in ODFS.
	dirName := "_odfs_crud"
	uniqueDirName := filemanager.GenerateTestFileName(dirName)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}

	if err := uiauto.Combine("Create folder in ODFS",
		files.OpenOneDrive(),
		files.CreateFolder(kb, uniqueDirName),
	)(ctx); err != nil {
		s.Fatal("Failed to create folder in ODFS: ", err)
	}
	defer os.Remove(filepath.Join(filemanager.FuseboxDirPath, odfsToken, uniqueDirName))

	// Check if we can navigate into the newly created folder.
	if err := files.OpenPath(filesapp.FilesTitlePrefix+filesapp.OneDrive, filesapp.OneDrive, uniqueDirName)(ctx); err != nil {
		s.Fatal("Failed to navigate into the newly created folder: ", err)
	}

	// Rename folder.
	renamedDirName := filemanager.GenerateTestFileName(dirName)
	if err := uiauto.Combine("Rename the folder",
		files.OpenOneDrive(),
		files.RenameFile(kb, uniqueDirName, renamedDirName),
		files.WaitUntilFileGone(uniqueDirName),
		files.WaitForFile(renamedDirName),
		files.OpenPath(filesapp.FilesTitlePrefix+filesapp.OneDrive, filesapp.OneDrive, renamedDirName),
	)(ctx); err != nil {
		s.Fatal("Failed to rename the folder: ", err)
	}
	defer os.Remove(filepath.Join(filemanager.FuseboxDirPath, odfsToken, renamedDirName))

	// Delete folder.
	if err := uiauto.Combine("Delete the folder",
		files.OpenOneDrive(),
		files.DeleteFileOrFolder(kb, renamedDirName),
	)(ctx); err != nil {
		s.Fatal("Failed to delete the folder: ", err)
	}
}
