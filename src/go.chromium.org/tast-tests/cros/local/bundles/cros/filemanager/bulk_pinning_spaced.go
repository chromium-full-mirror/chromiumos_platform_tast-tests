// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningSpaced,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the spaced CLI is called by Drivefs",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
			"fdegros@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_unstable",
		},
		Data: []string{
			"test_1KB.txt",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		Timeout:     5 * time.Minute,
		Fixture:     "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningSpaced(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	driveFsClient := fixt.DriveFs
	tconn := fixt.TestAPIConn

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Enable verbose logs so we can detect when spaced has been called by Drivefs.
	if err := fixt.DriveFs.WriteCommandLineFlags("--v=1"); err != nil {
		s.Fatal("Failed to write command line args: ", err)
	}
	if err := fixt.DriveFs.Restart(ctx); err != nil {
		s.Fatal("Failed waiting for DriveFS to restart: ", err)
	}

	// Start looking for Drivefs logs that indicate that spaced has been queried.
	drivefsLogReader, err := drivefs.NewLogReader(ctx, fixt.Chrome.NormalizedUser(), fixt.MountPath)
	if err != nil {
		s.Fatal("Failed to create Drivefs log reader: ", err)
	}
	matchCh, err := drivefsLogReader.WatchPattern(ctx, regexp.MustCompile(`file_system\.cc.*?Ignored line: `))
	if err != nil {
		s.Fatal("Failed to start watching Drivefs logs for pattern: ", err)
	}

	// Create a random file locally
	testFileName := filemanager.GenerateTestFileName(s.TestName() + ".txt")
	testFilePath := driveFsClient.MyDrivePath(testFileName)
	if err := fsutil.CopyFile(s.DataPath("test_1KB.txt"), testFilePath); err != nil {
		s.Fatal("Failed to copy test file: ", err)
	}
	// Cleanup: Remove the file locally
	defer os.Remove(testFilePath)

	// Verify that spaced has been called by Drivefs.
	select {
	case result := <-matchCh:
		if result.Err != nil {
			s.Fatal("Something went wrong waiting for log line: ", err)
		}
	case <-ctx.Done():
		s.Fatal("Timed out waiting for matching log line")
	}
}
