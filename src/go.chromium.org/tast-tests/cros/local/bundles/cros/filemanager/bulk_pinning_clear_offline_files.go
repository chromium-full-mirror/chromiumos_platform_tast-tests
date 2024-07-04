// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           BulkPinningClearOfflineFiles,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verify that when Clean offline storage is pressed, files are unpinned and made unavailable offline",
		BugComponent:   "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
			"fdegros@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
			"gaia",
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
			Value: "screenplay-a5c390c9-43ea-46c8-8fad-8337d7547f9f",
		}, {
			Key:   "feature_id",
			Value: "screenplay-8be5cf49-fd5e-4053-9f68-05c44d2ad5dd",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningClearOfflineFiles(ctx context.Context, s *testing.State) {
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

	// Create an existing file and enable bulk pinning.
	fileName := filemanager.GenerateTestFileName(s.TestName()) + ".txt"
	fileID, err := bulkpinning.CreateTestFile(ctx, apiClient, driveFsClient, s.DataPath("test_1KB.txt"), fileName)
	if err != nil {
		s.Fatal("Failed to create a test file: ", err)
	}
	defer func() {
		if err := apiClient.RemoveFileByID(cleanupCtx, fileID); err != nil {
			s.Log("Failed to remove file by ID: ", err)
		}
	}()

	testFilePath := driveFsClient.MyDrivePath(fileName)
	testFile, err := driveFsClient.NewFile(testFilePath)
	if err != nil {
		s.Fatal("Failed to create DriveFS file: ", err)
	}

	retryAction := func(actionToRetry uiauto.Action) uiauto.Action {
		// Retry the supplied action 10 times with a 2 second interval.
		return action.RetrySilently(10, actionToRetry, 2*time.Second)
	}

	if err := uiauto.Combine("toggle bulk pinning and ensure file gets appropriate attributes",
		// Enable bulk pinning from OS Settings.
		bulkpinning.EnableBulkPinningFromSettings(fixt.Chrome, tconn),
		// Wait for the test file to be available offline.
		retryAction(testFile.AvailableOfflineAction(true)),
		// Wait for the test file to be pinned.
		retryAction(testFile.PinnedAction(true)),
		// Clean up storage from OS Settings.
		bulkpinning.CleanUpStorageFromSettings(fixt.Chrome, tconn),
		// Wait for the test file to not be available offline.
		retryAction(testFile.AvailableOfflineAction(false)),
		// Wait for the test file to be unpinned.
		retryAction(testFile.PinnedAction(false)),
	)(ctx); err != nil {
		s.Fatal("Failed to clean up storage: ", err)
	}
}
