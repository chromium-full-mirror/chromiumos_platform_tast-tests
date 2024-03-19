// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoveDownloadsBindMountNoUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the removal of bind mount can migrate ~/Downloads to ~/MyFiles/Downloads without launching Chrome",
		BugComponent: "b:227525431",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"wenbojie@chromium.org",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		Data: []string{
			"test_1KB.txt",
		},
		Timeout: 5 * time.Minute,
	})
}

// RemoveDownloadsBindMountNoUI tests that ~/Downloads will be migrated to
// ~/MyFiles/Downloads correctly without launching Chrome. Steps:
//  1. Create user vault with normal bind mount.
//  2. Create a test file in ~/MyFiles/Downloads.
//  3. Restart cryptohomed with bind mount disabled.
//  4. Remount user again to check ~/Downloads is gone and test file under
//     ~/MyFiles/Downloads is untouched.
//  5. Restart cryptohomed with bind mount enabled.
//  6. Remount user again to check ~/Downloads is back and test file under
//     ~/MyFiles/Downloads is untouched.
func RemoveDownloadsBindMountNoUI(ctx context.Context, s *testing.State) {
	const (
		// Example user information
		fakeUser     = "fakeuser@example.com"
		fakePassword = "fakePassword"
	)

	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Start cryptohomed and wait for it to be available.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to start cryptohomed: ", err)
	}

	// Mount home directory for fake user.
	if err := cryptohome.RemoveVault(ctx, fakeUser); err != nil {
		s.Fatal("Failed to remove user vault before creating: ", err)
	}
	if err := cryptohome.CreateVault(ctx, fakeUser, fakePassword); err != nil {
		s.Fatal("Failed to create user vault: ", err)
	}
	defer func() {
		if err := cryptohome.UnmountVault(cleanUpCtx, fakeUser); err != nil {
			s.Error("Failed to unmount user vault: ", err)
		}
		if err := cryptohome.RemoveVault(cleanUpCtx, fakeUser); err != nil {
			s.Error("Failed to remove user vault: ", err)
		}
	}()

	// Get ~/Downloads and ~/MyFiles/Downloads folder path.
	userPath, err := cryptohome.UserPath(ctx, fakeUser)
	if err != nil {
		s.Fatal("Failed to get user folder path: ", err)
	}
	downloadsPath := filepath.Join(userPath, "Downloads")
	myFilesDownloadsPath := filepath.Join(userPath, "MyFiles", "Downloads")

	// Wait for user home directory to exist.
	if err := waitForDownloadsDir(ctx, myFilesDownloadsPath); err != nil {
		s.Fatal("Failed to wait for ~/MyFiles/Downloads folder: ", err)
	}

	// Assert ~/MyFiles/Downloads is a bind mount before migration.
	existBeforeMigration, err := cryptohome.DownloadsExistAsBindMount(ctx)
	if err != nil {
		s.Fatal("Failed to get mount information before migration: ", err)
	}
	if !existBeforeMigration {
		s.Fatal("Wrong state: Downloads bind mount is already removed")
	}

	// Copy a test file to ~/MyFiles/Downloads folder.
	testFilePath := filepath.Join(myFilesDownloadsPath, cryptohome.TestFile)
	if err := fsutil.CopyFile(s.DataPath(cryptohome.TestFile), testFilePath); err != nil {
		s.Fatalf("Failed to copy %q to %q: %v", cryptohome.TestFile, testFilePath, err)
	}

	// Read file content for content comparison later.
	fileContentBeforeMigration, err := os.ReadFile(testFilePath)
	if err != nil {
		s.Fatalf("Failed to read test file %q: %v", testFilePath, err)
	}

	// Unmount user vault before restarting cryptohomed.
	if err := cryptohome.UnmountVault(ctx, fakeUser); err != nil {
		s.Error("Failed to unmount user vault: ", err)
	}

	// Restart cryptohomed with no_downloads_bind_mount.
	if err := cryptohome.RestartCryptohomed(ctx, false /*enableDownloadsBindMount*/); err != nil {
		s.Fatal("Failed to restart cryptohomed for migration: ", err)
	}

	// Remount user vault.
	if err := cryptohome.MountVault(ctx, fakeUser, fakePassword); err != nil {
		s.Fatal("Failed to remount user: ", err)
	}

	// Wait for user home directory to exist.
	if err := waitForDownloadsDir(ctx, myFilesDownloadsPath); err != nil {
		s.Fatal("Failed to wait for ~/MyFiles/Downloads folder after remount: ", err)
	}

	// Check user.BindMountMigration xattr on ~/MyFiles/Downloads.
	result, err := testexec.CommandContext(ctx, "sudo", "getfattr", "-n", cryptohome.XattrName, myFilesDownloadsPath).Output()
	if err != nil {
		s.Fatalf("Failed to get xattr on %q: %v", myFilesDownloadsPath, err)
	}
	if !strings.Contains(string(result), fmt.Sprintf("%s=\"migrated\"", cryptohome.XattrName)) {
		s.Fatalf("Failed to migrate: Migrated xattr is missing from %q", myFilesDownloadsPath)
	}

	// Assert ~/Downloads doesn't exist any more.
	if _, err = os.Stat(downloadsPath); !os.IsNotExist(err) {
		s.Fatalf("Failed to migrate: %q still exists", downloadsPath)
	}

	// Assert there is no bind mount for Downloads.
	existAfterMigration, err := cryptohome.DownloadsExistAsBindMount(ctx)
	if err != nil {
		s.Fatal("Failed to get mount information after migration: ", err)
	}
	if existAfterMigration {
		s.Fatal("Failed to migrate: Downloads is still a bind mount")
	}

	// Verify test file content after migration.
	if err := cryptohome.VerifyFileContent(testFilePath, string(fileContentBeforeMigration)); err != nil {
		s.Fatal("Failed to migrate: test file content doesn't match after migration: ", err)
	}

	// Unmount user vault before restarting cryptohomed.
	if err := cryptohome.UnmountVault(ctx, fakeUser); err != nil {
		s.Error("Failed to unmount user vault: ", err)
	}

	// Restart cryptohomed with downloads bind mount ON.
	if err := cryptohome.RestartCryptohomed(ctx, true /*enableDownloadsBindMount*/); err != nil {
		s.Fatal("Failed to restart cryptohomed for restoration: ", err)
	}

	// Remount user vault.
	if err := cryptohome.MountVault(ctx, fakeUser, fakePassword); err != nil {
		s.Fatal("Failed to remount user: ", err)
	}

	// Wait for user home directory to exist.
	if err := waitForDownloadsDir(ctx, myFilesDownloadsPath); err != nil {
		s.Fatal("Failed to wait for ~/MyFiles/Downloads folder after remount: ", err)
	}

	// Assert ~/Downloads is restored.
	if _, err := os.Stat(downloadsPath); os.IsNotExist(err) {
		s.Fatalf("Failed to restore migration: %q is not restored", downloadsPath)
	}

	// Assert there is bind mount for Downloads.
	existAfterRestore, err := cryptohome.DownloadsExistAsBindMount(ctx)
	if err != nil {
		s.Fatal("Failed to get mount information after restoring migration: ", err)
	}
	if !existAfterRestore {
		s.Fatal("Failed to restore migration: Downloads is not a bind mount")
	}

	// Verify test file content after restoring migration.
	if err := cryptohome.VerifyFileContent(testFilePath, string(fileContentBeforeMigration)); err != nil {
		s.Fatal("Failed to restore migration: test file content doesn't match after restoring migration: ", err)
	}
}

func waitForDownloadsDir(ctx context.Context, path string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(path); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return err
	}
	return nil
}
