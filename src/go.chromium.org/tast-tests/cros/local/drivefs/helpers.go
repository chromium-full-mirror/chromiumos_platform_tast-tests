// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// PersistableToken derives the token from the mount path. This is used
// to identify the user account directory under ~/GCache/v2.
func PersistableToken(mountPath string) string {
	return strings.TrimPrefix(mountPath, "/media/fuse/drivefs-")
}

// ConfigPath returns the path to `elem...` in the DriveFS
// configuration directory based on the provided `homeDir` and
// `persistableToken`.
func ConfigPath(homeDir, persistableToken string, elem ...string) string {
	return path.Join(append([]string{homeDir, "GCache", "v2", persistableToken}, elem...)...)
}

// SaveDriveLogsOnError dumps the drivefs logs to the context output dir to
// enable ease of debugging when an error occurs. This should be deferred on any
// test that you wish to obtain logs on error.
func SaveDriveLogsOnError(ctx context.Context, hasError func() bool, normalizedUser, mountPath string) {
	if !hasError() {
		return
	}
	persistableToken := PersistableToken(mountPath)
	if len(persistableToken) == 0 {
		testing.ContextLog(ctx, "Could not obtain the drive persistable token from mount path: ", mountPath)
		return
	}
	homeDir, err := cryptohome.UserPath(ctx, normalizedUser)
	if err != nil {
		testing.ContextLog(ctx, "Could not obtain the home dir path: ", err)
		return
	}
	saveDriveLogs(ctx, homeDir, persistableToken)
}

func saveDriveLogs(ctx context.Context, homeDir, persistableToken string) {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Could not obtain the context out dir")
		return
	}

	contextDriveLogsDir := filepath.Join(outDir, "drivefs_logs")
	if err := os.Mkdir(contextDriveLogsDir, os.ModePerm); err != nil {
		testing.ContextLogf(ctx, "Could not create directory %q: %v", contextDriveLogsDir, err)
		return
	}
	driveFsLogPath := ConfigPath(homeDir, persistableToken, "Logs")
	files, err := os.ReadDir(driveFsLogPath)
	if err != nil {
		testing.ContextLogf(ctx, "Could not read Drive log directory %q: %v", driveFsLogPath, err)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filePath := filepath.Join(driveFsLogPath, file.Name())
		outputFilePath := filepath.Join(contextDriveLogsDir, file.Name())
		if err := fsutil.CopyFile(filePath, outputFilePath); err != nil {
			testing.ContextLogf(ctx, "Could not copy %q to %q: %v", filePath, outputFilePath, err)
		}
	}
}

// GenerateTestFileName generates a unique-ish file name based on a provided
// prefix, the current time, and a random number.
func GenerateTestFileName(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), rand.Intn(10000))
}

// MD5SumFile generates an MD5 sum of a file at `path`.
func MD5SumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// GoogleDSSWindowFinder is a finder for a window of Google DSS (Docs/Sheets/Slides)
// opened for the given fileName.
func GoogleDSSWindowFinder(fileName string) *nodewith.Finder {
	return nodewith.Role(role.Window).NameContaining(fileName + " - Google").HasClass("WebContentsViewAura")
}

// CloseAddAccountWindowIfNeededAndWaitFor can be used in the case where we
// want to wait for a specific element (e.g. window), but there might be a Add
// Account window appearing before the element (e.g. in Lacros).
// This method will close the Add Account window if it shows, and then wait
// for the element.
func CloseAddAccountWindowIfNeededAndWaitFor(tconn *chrome.TestConn, elementToWait *nodewith.Finder) uiauto.Action {
	return func(ctx context.Context) error {
		// Most of the interactions are with remote service, increase the timeout to 60s.
		ui := uiauto.New(tconn).WithTimeout(60 * time.Second)
		addAccountWindow := nodewith.Role(role.Window).NameContaining("Sign in to add a Google account").HasClass("WebContentsViewAura")
		closeButton := nodewith.Role(role.Button).Name("Close").HasClass("ImageButton")
		found, err := ui.FindAnyExists(ctx, addAccountWindow, elementToWait)
		if err != nil {
			return errors.Wrap(err, "failed to find either Add account window or the actual element to wait for")
		}
		// Found the Add Account window, close it.
		if found == addAccountWindow {
			return ui.LeftClickUntil(closeButton, ui.Exists(elementToWait))(ctx)
		}
		// Found the element directly.
		return nil
	}
}

// closeGoogleDSSWindow finds the Google Docs/Sheets/Slides window with the
// specific file name and close it.
func closeGoogleDSSWindow(ctx context.Context, tconn *chrome.TestConn, fileName string) error {
	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
		return strings.Contains(w.Title, fileName) && strings.Contains(w.Title, "Google")
	})
	if err != nil {
		return errors.Wrap(err, "failed to find the Google Drive window to close")
	}
	if err := w.CloseWindow(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to close the Google Drive window")
	}
	return nil
}

// waitForDSSWindowAndClose waits for the Google Docs/Sheets/Slides window to open and close it.
// If "ignoreCloseError" is true, it will just ignore the error, otherwise it returns error if the closure fails,.
func waitForDSSWindowAndClose(tconn *chrome.TestConn, fileName string, ignoreCloseError bool) uiauto.Action {
	return func(ctx context.Context) error {
		if err := CloseAddAccountWindowIfNeededAndWaitFor(tconn, GoogleDSSWindowFinder(fileName))(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for the DSS window")
		}
		if err := closeGoogleDSSWindow(ctx, tconn, fileName); err != nil {
			if !ignoreCloseError {
				return errors.Wrap(err, "failed to close the DSS window")
			}
		}
		return nil
	}
}

// WaitForDSSWindowAndClose closes the DSS window and guarantees that
// the window is closed successfully, it will return errors if the closure fails.
func WaitForDSSWindowAndClose(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return waitForDSSWindowAndClose(tconn, fileName, false /*=ignoreCloseError*/)
}

// WaitForDSSWindowAndMaybeClose tries to close the DSS window but won't
// return error if it fails. This is used mostly when closing window is the last
// step.
func WaitForDSSWindowAndMaybeClose(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return waitForDSSWindowAndClose(tconn, fileName, true /*=ignoreCloseError*/)
}

// VerifySourceDestinationMD5SumMatch use MD5 to check if the file content
// uploaded to Google Drive is the same with the local file.
func VerifySourceDestinationMD5SumMatch(driveFsClient *DriveFs, srcFilePath, fileName string) uiauto.Action {
	return func(ctx context.Context) error {
		// Wait for file to be available locally
		driveFilePath := driveFsClient.MyDrivePath(fileName)
		driveFile, err := driveFsClient.NewFile(driveFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to build DriveFS file")
		}
		err = action.RetrySilently(5, driveFile.ExistsAction(), 1*time.Second)(ctx)
		if err != nil {
			return errors.Wrap(err, "file not available locally")
		}

		// Now compare the uploaded data with what we have locally
		md5SumForLocalFile, err := MD5SumFile(srcFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to checksum for local file")
		}
		md5SumForDriveFsFile, err := MD5SumFile(driveFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to checksum for DriveFS file")
		}

		if !strings.EqualFold(md5SumForDriveFsFile, md5SumForLocalFile) {
			return errors.Errorf("Checksum mismatch! Got: %v Expected: %v", md5SumForDriveFsFile, md5SumForLocalFile)
		}
		return nil
	}
}
