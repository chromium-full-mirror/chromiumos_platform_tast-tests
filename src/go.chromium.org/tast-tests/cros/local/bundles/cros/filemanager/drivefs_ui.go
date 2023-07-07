// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
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
		Fixture: "driveFsStarted",
	})
}

func DrivefsUI(ctx context.Context, s *testing.State) {
	const testFileName = "drivefs"

	fixt := s.FixtValue().(*drivefs.FixtureData)
	cr := fixt.Chrome
	tconn := fixt.TestAPIConn
	mountPath := fixt.MountPath
	cleanupCtx := ctx

	// Create a test file inside Drive.
	drivefsRoot := filepath.Join(mountPath, "root")
	testFile, err := os.Create(filepath.Join(drivefsRoot, testFileName))
	if err != nil {
		s.Fatalf("Failed to create test file inside %q: %v", drivefsRoot, err)
	}
	testFile.Close()
	// Don't delete the test file after the test as there may not be enough time
	// after the test for the deletion to be synced to Drive.

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer drivefs.SaveDriveLogsOnError(ctx, s.HasError, cr.NormalizedUser(), mountPath)

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
