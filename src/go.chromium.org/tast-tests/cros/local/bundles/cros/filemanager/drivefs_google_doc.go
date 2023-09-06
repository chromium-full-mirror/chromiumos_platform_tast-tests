// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DrivefsGoogleDoc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that a google doc created via Drive API syncs to DriveFS",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"austinct@chromium.org",
			"benreich@chromium.org",
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
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStarted",
	})
}

func DrivefsGoogleDoc(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	APIClient := fixt.APIClient
	tconn := fixt.TestAPIConn
	dfs := fixt.DriveFs

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Create a blank Google doc in the root GDrive directory.
	testDocFileName := filemanager.GenerateTestFileName(s.TestName())
	file, err := APIClient.CreateBlankGoogleDoc(ctx, testDocFileName, []string{"root"})
	if err != nil {
		s.Fatal("Failed creating blank google doc: ", err)
	}
	defer APIClient.RemoveFileByID(cleanupCtx, file.Id)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	testFileNameWithExt := fmt.Sprintf("%s.gdoc", testDocFileName)
	testFilePath := dfs.MyDrivePath(testFileNameWithExt)
	testFile, err := dfs.NewFile(testFilePath)

	// Wait 4 minutes for the file to appear.
	if err = action.RetrySilently(48, testFile.ExistsAction(), 5*time.Second)(ctx); err != nil {
		s.Fatal("Failed waiting for test file to be available locally: ", err)
	}

	// Launch Files App and check that Drive is accessible.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed launching Files app: ", err)
	}

	if err := uiauto.Combine("open files app and wait for file to appear",
		filesApp.OpenDrive(),
		filesApp.WithTimeout(30*time.Second).WaitForFile(testFileNameWithExt),
	)(ctx); err != nil {
		s.Fatalf("Failed waiting for the test file %q to appear in Drive: %v", testFileNameWithExt, err)
	}
}
