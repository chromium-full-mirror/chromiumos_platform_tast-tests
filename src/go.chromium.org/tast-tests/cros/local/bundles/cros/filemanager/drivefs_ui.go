// Copyright 2020 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DrivefsUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that drivefs can be accessed through the UI",
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
			"group:drivefs-cq",
			"group:hw_agnostic",
			"group:mainline",
		},
		Params: []testing.Param{{
			Name:    "fieldtrial_testing_config_off",
			Fixture: "driveFsStartedFieldTrialDisabled",
		}, {
			Name:    "fieldtrial_testing_config_on",
			Fixture: "driveFsStartedFieldTrialEnabled",
		}},
		Timeout: 5 * time.Minute,
	})
}

func DrivefsUI(ctx context.Context, s *testing.State) {
	const testFileName = "drivefs"

	fixt := s.FixtValue().(*drivefs.FixtureData)
	tconn := fixt.TestAPIConn
	dfs := fixt.DriveFs

	// Create a test file inside Drive.
	testFilePath := dfs.MyDrivePath(testFileName)
	testFile, err := os.Create(testFilePath)
	if err != nil {
		s.Fatalf("Failed to create test file at %q: %v", testFilePath, err)
	}
	testFile.Close()
	// Don't delete the test file after the test as there may not be enough time
	// after the test for the deletion to be synced to Drive.

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	// Launch Files App.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	if err := uiauto.Combine("check Drive",
		// Open the Google Drive folder and check for the test file.
		files.OpenDrive(),
		// Wait for the file, if it can't find it try to maximize the window and find again.
		files.PerformActionAndRetryMaximizedOnFail(files.WaitForFile(testFileName)),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for the test file in Drive: ", err)
	}
}
