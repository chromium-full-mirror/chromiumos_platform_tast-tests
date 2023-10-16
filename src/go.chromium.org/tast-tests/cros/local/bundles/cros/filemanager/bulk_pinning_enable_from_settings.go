// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningEnableFromSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the bulk pinning feature can be enabled via settings",
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
			"cbx_stable",
		},
		Data: []string{
			"test_1KB.txt",
		},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-a76e7f4a-57cc-4dc7-97ff-e076f5a7d416",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-6c9f21dc-095d-40e0-a86a-b18ebcb8e6df",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-1730ae02-2719-4e5e-8409-e279e32e44d5",
			},
		},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningEnableFromSettings(ctx context.Context, s *testing.State) {
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

	// Upload an existing file to Drive.
	existingFileName := filemanager.GenerateTestFileName("existing") + ".txt"
	existingFileID, err := bulkpinning.CreateTestFile(ctx, apiClient, driveFsClient, s.DataPath("test_1KB.txt"), existingFileName)
	if err != nil {
		s.Fatal("Failed to create a test file: ", err)
	}
	defer func() {
		if err := apiClient.RemoveFileByID(cleanupCtx, existingFileID); err != nil {
			s.Log("Failed to remove file by ID: ", err)
		}
	}()

	// Create a local file in My Drive before enabling bulk pinning.
	testLocalFileName1 := "test_1KB.txt"
	testLocalFilePath1 := driveFsClient.MyDrivePath(testLocalFileName1)
	if err := fsutil.CopyFile(s.DataPath(testLocalFileName1), testLocalFilePath1); err != nil {
		s.Fatalf("Cannot copy %q to %q: %v", testLocalFileName1, testLocalFilePath1, err)
	}
	defer os.Remove(testLocalFilePath1)

	// Enable bulk pinning.
	if err := uiauto.Combine("enable bulk pinning and ensure file is pinned",
		bulkpinning.EnableBulkPinningFromSettings(fixt.Chrome, tconn),
		bulkpinning.AssertFileAvailableOffline(tconn, existingFileName, 15*time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed to enable bulk pinning: ", err)
	}

	// Create a second local file in My Drive after enabling bulk pinning.
	testLocalFileName2 := "test_1KB2.txt"
	testLocalFilePath2 := driveFsClient.MyDrivePath(testLocalFileName2)
	if err := fsutil.CopyFile(s.DataPath(testLocalFileName1), testLocalFilePath2); err != nil {
		s.Fatalf("Cannot copy %q to %q: %v", testLocalFileName1, testLocalFilePath2, err)
	}
	defer os.Remove(testLocalFilePath2)

	// Add a new file to Drive and ensure it shows up in Files.
	newFileName := filemanager.GenerateTestFileName("new") + ".txt"
	newFileID, err := bulkpinning.CreateTestFile(ctx, apiClient, driveFsClient, s.DataPath("test_1KB.txt"), newFileName)
	if err != nil {
		s.Fatal("Failed to create a test file: ", err)
	}
	defer func() {
		if err := apiClient.RemoveFileByID(cleanupCtx, newFileID); err != nil {
			s.Log("Failed to remove file by ID: ", err)
		}
	}()
	if err = bulkpinning.AssertFileAvailableOffline(tconn, newFileName, 15*time.Second)(ctx); err != nil {
		s.Fatal("Failed to make new file available offline: ", err)
	}

	// Verify that both locally created files are made "available offline" after enabling bulk pinning.
	if err := uiauto.Combine("enable bulk pinning and ensure file is pinned",
		bulkpinning.AssertFileAvailableOffline(tconn, testLocalFileName1, 15*time.Second),
		bulkpinning.AssertFileAvailableOffline(tconn, testLocalFileName2, 15*time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed to assert the locally created files are available offline: ", err)
	}
}
