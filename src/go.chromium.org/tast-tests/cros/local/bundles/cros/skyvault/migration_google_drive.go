// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MigrationGoogleDrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies that local files are moved to Google Drive, when LocalUserFilesAllowed and LocalUserFilesMigrationDestination policies are set",
		BugComponent:   "b:1533988",
		Contacts: []string{
			"poromov@chromium.org",
			"aidazolic@chromium.org",
			"cros-commercial-clippy-eng@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"gaia",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		Fixture: "driveFsManagedWithSkyVaultGA",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DriveDisabled{}, pci.Served),
			pci.SearchFlag(&policy.LocalUserFilesAllowed{}, pci.Served),
			pci.SearchFlag(&policy.LocalUserFilesMigrationDestination{}, pci.Served),
		},
	})
}

// MigrationGoogleDrive tests that local files are uploaded to Google Drive when forced by policy.
func MigrationGoogleDrive(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	tconn := fixt.TestAPIConn
	cr := fixt.Chrome
	dfs := fixt.DriveFs
	driveAPIClient := fixt.APIClient

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "migration_google_drive")
	s.AttachErrorHandlers(handler, handler)

	const testFile = "testFile.txt"
	if err := createTestFile(ctx, cr, testFile); err != nil {
		s.Fatal("Failed to create a test file: ", err)
	}
	defer deleteTestFile(cleanupCtx, cr, testFile)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	// TODO(crbug.com/372897049): Get the device ID to construct the full folder name,
	// so that different test runs don't interfere with each other.
	cloudFolderName := regexp.MustCompile("^ChromeOS device ")
	if err := deleteAllUploadFoldersGoogleDrive(ctx, files, cloudFolderName, testFile, dfs, driveAPIClient); err != nil {
		s.Error("Failed to clean up Drive: ", err)
	}

	ui := uiauto.New(tconn)
	dialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("Your file upload to Google Drive *"))
	uploadNowButton := nodewith.Ancestor(dialog).Role(role.Button).Name("Upload now")

	if err := uiauto.Combine("Upload now",
		uiauto.Log("Starting Upload now step"),
		ui.WaitUntilExists(dialog),
		ui.DoDefault(uploadNowButton),
		ui.WaitUntilGone(dialog),
	)(ctx); err != nil {
		s.Fatal("Failed to click Upload now: ", err)
	}

	// Verify that the migration completed.
	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitleContains("Files were successfully uploaded to")); err != nil {
		s.Error("Failed to wait for migration completed notification: ", err)
	}

	if err := uiauto.Combine("Check that files are uploaded to Google Drive",
		uiauto.Log("Starting Check uploaded files"),
		files.OpenDrive(),
		files.OpenFile(cloudFolderName),
		files.WithTimeout(10*time.Second).WaitForFile(testFile),
	)(ctx); err != nil {
		s.Error("Failed to find the uploaded files: ", err)
	}

	testing.ContextLog(ctx, "Removing files")
	files.OpenDrive()(ctx)
	exactFolderName, err := files.WaitForFileByPattern(ctx, cloudFolderName)
	if err == nil {
		cleanUploads(ctx, exactFolderName, testFile, dfs, driveAPIClient)
	}
}

// createTestFile creates a local file in MyFiles.
func createTestFile(ctx context.Context, cr *chrome.Chrome, testFile string) error {
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to retrieve user's MyFiles path")
	}

	filePath := filepath.Join(myFilesPath, testFile)
	f, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to create a test file")
	}
	if _, err := f.WriteString("TestData"); err != nil {
		os.Remove(filePath)
		return errors.Wrap(err, "failed to write to test file")
	}
	if err := os.Chown(filePath, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		os.Remove(filePath)
		return errors.Wrap(err, "failed to chown a test file")
	}
	return nil
}

// deleteTestFile deletes the previously created local file in MyFiles.
func deleteTestFile(ctx context.Context, cr *chrome.Chrome, testFile string) {
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		testing.ContextLog(ctx, "Failed to retrieve user's MyFiles path")
		return
	}

	filePath := filepath.Join(myFilesPath, testFile)
	os.Remove(filePath)
}

// deleteAllUploadFoldersGoogleDrive deletes all folders matching the given cloudFolderName pattern
// in the Files app.
func deleteAllUploadFoldersGoogleDrive(ctx context.Context, files *filesapp.FilesApp, cloudFolderName *regexp.Regexp, testFile string, dfs *drivefs.DriveFs, driveAPIClient *drivefs.APIClient) error {
	if err := files.OpenDrive()(ctx); err != nil {
		return errors.Wrap(err, "failed to open Drive")
	}

	for {
		folderName, err := files.WithTimeout(5*time.Second).WaitForFileByPattern(ctx, cloudFolderName)
		if err != nil { // No folders matching the pattern exist, exit the loop
			break
		}

		cleanUploads(ctx, folderName, testFile, dfs, driveAPIClient)
	}
	return nil
}

// cleanUploads deletes the folder with the test file in it.
func cleanUploads(ctx context.Context, folder, testFile string, dfs *drivefs.DriveFs, driveAPIClient *drivefs.APIClient) {
	strSlice := []string{folder, testFile}
	file := strings.Join(strSlice, "/")
	testing.ContextLogf(ctx, "Deleting file %s", file)

	deleteFileOrFolderDrive(ctx, file, dfs, driveAPIClient)

	testing.ContextLogf(ctx, "Deleting folder %s", folder)
	deleteFileOrFolderDrive(ctx, folder, dfs, driveAPIClient)
}

// deleteFileOrFolderDrive deletes the given file or folder from Google Drive.
func deleteFileOrFolderDrive(ctx context.Context, filePath string, dfs *drivefs.DriveFs, driveAPIClient *drivefs.APIClient) {
	driveFilePath := dfs.MyDrivePath(filePath)
	if err := os.Remove(driveFilePath); err != nil {
		testing.ContextLogf(ctx, "Failed to remove %s: %v", filePath, err)
	}
	if err := drivefs.RemoveDriveFsFileViaAPI(dfs, driveAPIClient, filePath)(ctx); err != nil {
		testing.ContextLogf(ctx, "Failed to remove %s via Drive API: %v", filePath, err)
	}
}
