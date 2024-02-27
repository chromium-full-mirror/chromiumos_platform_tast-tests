// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/network"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningBlobsAvailableOffline,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that after bulk pinning enabled, blobs can be opened offline",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_stable",
		},
		Data: []string{
			"test_1KB.txt",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-4c179850-5b61-4fd0-85a6-271a83c3c468",
		}, {
			Key:   "feature_id",
			Value: "screenplay-cb3e1d67-a0a8-4c73-a824-357d56f729fa",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabledWithLogLevelFine",
	})
}

func BulkPinningBlobsAvailableOffline(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	apiClient := fixt.APIClient
	driveFsClient := fixt.DriveFs

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Create an existing text file and enable bulk pinning.
	createdFileName := filemanager.GenerateTestFileName(s.TestName()) + ".txt"
	testFileDataPath := s.DataPath("test_1KB.txt")
	existingFileID, err := bulkpinning.CreateTestFile(ctx, apiClient, driveFsClient, testFileDataPath, createdFileName)
	if err != nil {
		s.Fatal("Failed to create a test file: ", err)
	}
	defer func() {
		if err := apiClient.RemoveFileByID(cleanupCtx, existingFileID); err != nil {
			s.Log("Failed to remove file by ID: ", err)
		}
	}()

	if err := uiauto.Combine("enable bulk pinning and ensure file is pinned",
		bulkpinning.EnableBulkPinningFromSettings(fixt.Chrome, tconn),
		bulkpinning.AssertFileAvailableOffline(tconn, createdFileName, 15*time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed to enable bulk pinning: ", err)
	}

	want, err := os.ReadFile(testFileDataPath)
	if err != nil {
		s.Fatalf("Failed to open test file %q: %v", testFileDataPath, err)
	}
	filePathToCheck := driveFsClient.MountPath("root", createdFileName)
	s.Log("Attempting to check file contents when offline")
	if err := network.ExecFuncOnChromeOffline(ctx, verifyFileContents(string(want), filePathToCheck)); err != nil {
		s.Fatal("Failed verifying files content are available offline: ", err)
	}
}

// verifyFileContents verifies the contents supplied by `want` are the same
// contents that are at the `filePathToCheck`.
func verifyFileContents(want, filePathToCheck string) uiauto.Action {
	return func(ctx context.Context) error {
		got, err := os.ReadFile(filePathToCheck)
		if err != nil {
			return errors.Wrapf(err, "failed to open %q", filePathToCheck)
		}
		if !strings.EqualFold(string(got), want) {
			return errors.Errorf("failed comparing expected file to offline file; got %q want %q", got, want)
		}
		testing.ContextLog(ctx, "Successfully verified file contents")
		return nil
	}
}
