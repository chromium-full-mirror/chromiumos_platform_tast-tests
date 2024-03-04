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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoveDownloadsBindMount,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the removal of bind mount can migrate ~/Downloads to ~/MyFiles/Downloads",
		BugComponent: "b:227525431",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"wenbojie@chromium.org",
		},
		SoftwareDeps: []string{"chrome"},
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

// RemoveDownloadsBindMount tests that ~/Downloads will be migrated to
// ~/MyFiles/Downloads correctly. Steps:
//  1. Log in with normal Downloads bind mount.
//  2. Create a test file in ~/MyFiles/Downloads.
//  3. Log out and restart cryptohomed with bind mount disabled.
//  4. Log in again to check ~/Downloads is gone and test file under
//     ~/MyFiles/Downloads is untouched.
//  5. Log out and restart cryptohomed with bind mount enabled.
//  6. Check ~/Downloads is back and test file under ~/MyFiles/Downloads
//     is untouched.
func RemoveDownloadsBindMount(ctx context.Context, s *testing.State) {
	// We nee this flag to see if the restore is successful so we can use it to
	// see if cleanup is needed or not.
	restoreSuccess := false

	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Login to Chrome.
	cr1, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to login Chrome: ", err)
	}
	defer func() {
		if cr1 == nil {
			return
		}
		if err := cr1.Close(cleanUpCtx); err != nil {
			testing.ContextLog(cleanUpCtx, "Failed to close Chrome: ", err)
		}
	}()

	// Get ~/Downloads and ~/MyFiles/Downloads folder path.
	userPath, err := cryptohome.UserPath(ctx, cr1.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user folder path: ", err)
	}
	downloadsPath := filepath.Join(userPath, "Downloads")
	myFilesDownloadsPath := filepath.Join(userPath, "MyFiles", "Downloads")

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

	// Logging out to update cryptohomed.
	tconn1, err := cr1.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection for logout: ", err)
	}
	if err := quicksettings.SignOut(ctx, tconn1); err != nil {
		s.Fatal("Failed to sign out with quick settings: ", err)
	}

	// Restart cryptohomed with downloads bind mount OFF.
	if err := cryptohome.RestartCryptohomed(ctx, false /*enableDownloadsBindMount*/); err != nil {
		s.Fatal("Failed to restart cryptohomed for migration: ", err)
	}

	// Logging back in to check migration result.
	cr2, err := chrome.New(
		ctx,
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to re-login chrome: ", err)
	}
	defer func() {
		if cr2 == nil {
			return
		}
		if err := cr2.Close(cleanUpCtx); err != nil {
			testing.ContextLog(cleanUpCtx, "Failed to close Chrome: ", err)
		}
	}()

	// Check user.BindMountMigration xattr on ~/MyFiles/Downloads.
	result, err := testexec.CommandContext(ctx, "sudo", "getfattr", "-n", cryptohome.XattrName, myFilesDownloadsPath).Output()
	if err != nil {
		s.Fatalf("Failed to get xattr on %q: %v", myFilesDownloadsPath, err)
	}
	if !strings.Contains(string(result), fmt.Sprintf("%s=\"migrated\"", cryptohome.XattrName)) {
		s.Fatalf("Failed to migrate: Migrated xattr is missing from %q", myFilesDownloadsPath)
	}

	// Cleanup: remove xattr.
	defer func() {
		// The restore process will remove xattr, so no need to cleanup.
		if restoreSuccess {
			return
		}
		if err := testexec.CommandContext(cleanUpCtx, "sudo", "setfattr", "-x", cryptohome.XattrName, myFilesDownloadsPath).Run(testexec.DumpLogOnError); err != nil {
			testing.ContextLog(cleanUpCtx, "Failed to remove xattr: ", err)
		}
	}()

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

	// Logging out to update cryptohomed.
	tconn2, err := cr2.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection for logout (2nd): ", err)
	}
	if err := quicksettings.SignOut(ctx, tconn2); err != nil {
		s.Fatal("Failed to sign out with quick settings (2nd): ", err)
	}

	// Restart cryptohomed with downloads bind mount ON.
	if err := cryptohome.RestartCryptohomed(ctx, true /*enableDownloadsBindMount*/); err != nil {
		s.Fatal("Failed to restart cryptohomed for restoration: ", err)
	}

	// Logging back in to check migration restoration.
	cr3, err := chrome.New(
		ctx,
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to re-login chrome (2nd): ", err)
	}
	defer func() {
		if cr3 == nil {
			return
		}
		if err := cr3.Close(cleanUpCtx); err != nil {
			testing.ContextLog(cleanUpCtx, "Failed to close Chrome: ", err)
		}
	}()

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

	restoreSuccess = true
}
