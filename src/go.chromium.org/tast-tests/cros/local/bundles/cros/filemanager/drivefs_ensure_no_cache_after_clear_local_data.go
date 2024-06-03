// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DrivefsEnsureNoCacheAfterClearLocalData,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that a file created on the cloud can still be read after clearing the local data",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@chromium.org",
			"travislane@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
			"gaia",
		},
		Attr: []string{
			"group:mainline",
			"group:drivefs-cq",
			"group:hw_agnostic",
			"informational",
		},
		Data: []string{
			"test_1KB.txt",
		},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStarted",
	})
}

// DrivefsEnsureNoCacheAfterClearLocalData is the actual test.
func DrivefsEnsureNoCacheAfterClearLocalData(ctx context.Context, s *testing.State) {
	const (
		retryAttempts = 20
		retryInterval = 5 * time.Second
	)
	fixt := s.FixtValue().(*drivefs.FixtureData)
	cr := fixt.Chrome
	apiClient := fixt.APIClient
	driveFsClient := fixt.DriveFs

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	// Create a random file locally and ensure it gets uploaded.
	testFileName := filemanager.GenerateTestFileName(s.TestName() + ".txt")
	driveFile, err := apiClient.CreateFileFromLocalFile(ctx,
		testFileName, "root", s.DataPath("test_1KB.txt"))
	if err != nil {
		s.Fatal("Failed to create test file: ", err)
	}
	// Cleanup: Remove the file on the cloud
	defer apiClient.RemoveFileByID(cleanupCtx, driveFile.Id)

	// Wait for file to be available locally
	testFilePath := driveFsClient.MyDrivePath(testFileName)
	testFile, err := driveFsClient.NewFile(testFilePath)
	if err != nil {
		s.Fatal("Failed to build DriveFS file: ", err)
	}
	err = action.RetrySilently(retryAttempts, testFile.ExistsAction(), retryInterval)(ctx)
	if err != nil {
		s.Fatal("Failed to wait for file to be available locally: ", err)
	}

	// Read the text of the test file to compare against the values at the mount
	// point.
	wantBytes, err := os.ReadFile(s.DataPath("test_1KB.txt"))
	if err != nil {
		s.Fatalf("Failed to open %q: %v", s.DataPath("test_1KB.txt"), err)
	}
	want := string(wantBytes)

	// Read the test file in the mount point and compare it to the supplied test
	// file, they should match. For the chrome networking stack this should not
	// cache the request by the etag.
	if err := readFileAndCompare(testFilePath, want); err != nil {
		s.Fatal("Failed to read file and compare contents: ", err)
	}

	// Clear the local data from the chrome://drive-internals page.
	if err := driveFsClient.ClearLocalData(ctx, cr); err != nil {
		s.Fatal("Failed to clear local data: ", err)
	}

	// Attempt to read the file again after clearing the data.
	if err := readFileAndCompare(testFilePath, want); err != nil {
		s.Fatal("Failed to read file and compare contents: ", err)
	}
}

// readFileAndCompare opens the supplied `path` and reads the contents and
// compares it against `want`.
func readFileAndCompare(path, want string) error {
	if got, err := os.ReadFile(path); err != nil {
		return errors.Wrapf(err, "failed to open %q", path)
	} else if string(got) != string(want) {
		return errors.Errorf("unexpected output, got = %q, want %q", string(got), string(want))
	}
	return nil
}
