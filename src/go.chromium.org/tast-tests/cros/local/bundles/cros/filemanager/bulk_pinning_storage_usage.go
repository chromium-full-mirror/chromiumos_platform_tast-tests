// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningStorageUsage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that offline storage usage is increased after enabling bulk pinning",
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
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-d4f9911f-ee99-4af9-a760-124e63457f31",
		}},
		Data: []string{
			"test_1KB.txt",
		},
		// Creating up to 10 files could take up to 5min.
		// Waiting up to 10 files offline could take up to 5min.
		// Needs 2min for deleting the created files.
		// 3min for other actions.
		Timeout: 15 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningStorageUsage(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	driveFsClient := fixt.DriveFs
	cr := fixt.Chrome

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	offlineSizeBefore, driveSizeBefore, err := recordOfflineSize(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to get offline disk usage before enabling bulk pinning: ", err)
	}

	// Create test files.
	testFiles, err := createFiles(ctx, driveFsClient, s.TestName(), s.DataPath("test_1KB.txt"))
	if err != nil {
		s.Fatal("Failed to create test files: ", err)
	}

	if err := makeOffline(ctx, cr, tconn, testFiles); err != nil {
		s.Fatal("Failed to make the test files offline: ", err)
	}

	offlineSizeAfter, driveSizeAfter, err := recordOfflineSize(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to get offline disk usage after enabling bulk pinning: ", err)
	}

	expected := offlineSizeBefore + uint64(len(testFiles)*1024)
	if offlineSizeAfter < expected {
		s.Fatalf("Failed to verify os offline usage, got %s, want %s",
			strconv.FormatUint(offlineSizeAfter, 10),
			strconv.FormatUint(expected, 10))
	}
	expected = driveSizeBefore + uint64(len(testFiles)*1024)
	if driveSizeAfter < expected {
		s.Fatalf("Failed to verify os offline usage, got %s, want %s",
			strconv.FormatUint(driveSizeAfter, 10),
			strconv.FormatUint(expected, 10))
	}
}

func recordOfflineSize(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (uint64, uint64, error) {
	storagePage := "storage"
	storageFinder := nodewith.NameStartingWith("Storage management").Role(role.StaticText)
	offlineFiles := nodewith.NameRegex(regexp.MustCompile("Offline files [0-9]"))
	offlineSize, err := getOfflineUsage(ctx, cr, tconn, storagePage, storageFinder, offlineFiles)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get OS storage usage")
	}

	drivePage := "googleDrive"
	offlineUsing := nodewith.NameStartingWith("Using ").Role(role.StaticText).First()
	driveSize, err := getOfflineUsage(ctx, cr, tconn, drivePage, bulkpinning.SettingsToggleFinder, offlineUsing)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get drive storage usage")
	}

	return offlineSize, driveSize, nil
}

// getOfflineUsage returns offline disk uage if no error.
func getOfflineUsage(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn,
	settingsPage string, settingsFinder, usageFinder *nodewith.Finder) (uint64, error) {
	ui := uiauto.New(tconn)

	// Open Settings page at storage management.
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, settingsPage, ui.Exists(settingsFinder)); err != nil {
		return 0, errors.Wrapf(err, "failed to launch settings page at %s", settingsPage)
	}

	// Get offline files size.
	nodeInfo, err := ui.Info(ctx, usageFinder)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get offline files node info")
	}

	s := strings.Split(nodeInfo.Name, " ")
	if len(s) < 2 {
		return 0, errors.Errorf("the format of the disk size is wrong, got %s, want x B/KB/MB/GB", nodeInfo.Name)
	}
	return getSize(s[len(s)-2], s[len(s)-1])
}

func getSize(number, unit string) (uint64, error) {
	if strings.Contains(number, ",") {
		s := strings.Split(number, ",")
		number = strings.Join(s, "")
	}
	size, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse the disk size, got %s, want x B/KB/MB/GB", number)
	}

	if unit == "B" {
		return uint64(size), nil
	}
	if unit == "KB" {
		return uint64(size * 1024), nil
	}
	if unit == "MB" {
		return uint64(size * (1024 ^ 2)), nil
	}
	if unit == "GB" {
		return uint64(size * (1024 ^ 3)), nil
	}
	return 0, errors.Errorf("the format of the disk size is wrong, got %s, want B/KB/MB/GB", unit)
}

// createFiles create some files in driveFs from a local file.
func createFiles(ctx context.Context, driveFsClient *drivefs.DriveFs, testName, fileDataPath string) ([]*drivefs.File, error) {
	// Create some files.
	// The number of files is randomly between 1 and 10.
	// The size of each file is 1KB.
	// The total size of the new files will be numberOfFiles*KB.
	numberOfFiles := rand.Intn(10) + 1
	testing.ContextLogf(ctx, "Creating %d files", numberOfFiles)
	var testFiles []*drivefs.File
	for i := 0; i < numberOfFiles; i++ {
		testFileName := filemanager.GenerateTestFileName(fmt.Sprintf("%s-%d", testName, i)) + ".txt"
		testFilePath := driveFsClient.MyDrivePath(testFileName)
		if err := fsutil.CopyFile(fileDataPath, testFilePath); err != nil {
			return nil, errors.Wrapf(err, "failed to copy test file %q", testFilePath)
		}
		testFile, err := driveFsClient.NewFile(testFilePath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create DriveFS file")
		}
		testFiles = append(testFiles, testFile)
	}
	// Cello uploads 10 files concurrently, so move all the files first, then wait
	// for them to upload to take advantage of the concurrency.
	for i, testFile := range testFiles {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if uncommited, err := testFile.IsUncommitted(); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to wait for file to upload"))
			} else if uncommited {
				return errors.New("file is still not uploaded")
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
			return nil, errors.Wrap(err, "failed waiting foe file to upload")
		}
		testing.ContextLogf(ctx, "File %d has no uncommitted data", i)
	}
	return testFiles, nil
}

// removeFiles removes all files created by this test.
func removeFiles(ctx context.Context, apiClient *drivefs.APIClient, fileIDs []string) {
	for _, fileID := range fileIDs {
		if err := apiClient.RemoveFileByID(ctx, fileID); err != nil {
			testing.ContextLog(ctx, "Failed to remove file by ID: ", err)
		}
	}
}

// makeOffline does the following things:
// 1. enable bulk pinning from settings page.
// 2. check every file available offline and pinned.
func makeOffline(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, testFiles []*drivefs.File) error {
	if err := bulkpinning.EnableBulkPinningFromSettings(cr, tconn)(ctx); err != nil {
		return errors.Wrap(err, "failed to enable bulk pinning")
	}

	for _, testFile := range testFiles {
		if err := action.RetrySilently(100, uiauto.Combine("available offline and pinned",
			testFile.AvailableOfflineAction(true),
			testFile.PinnedAction(true),
		), time.Second)(ctx); err != nil {
			return errors.Wrapf(err, "failed to check %s offline", testFile.Name())
		}
	}
	return nil
}
