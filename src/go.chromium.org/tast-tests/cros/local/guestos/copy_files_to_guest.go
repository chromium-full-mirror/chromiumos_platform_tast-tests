// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CopyFilesToGuest tests copying files into the guest via the Files app sharing mechanism
func CopyFilesToGuest(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, guest vm.Guest, dirName string) error {
	if err := guest.Cleanup(ctx, "."); err != nil {
		return errors.Wrap(err, "failed to cleanup the home directory before the test")
	}

	// Open Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to open Files app")
	}

	testFiles := []string{"testfile1.txt", "testfile2.txt", "testfile3.txt"}
	testing.ContextLog(ctx, "Test copying files to guest")

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}

	// Create some files in Downloads.
	for _, file := range testFiles {
		path := filepath.Join(downloadsPath, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			return errors.Wrap(err, "failed to create file in Downloads")
		}
		defer os.Remove(path)
	}

	// Copy files from Downloads to guest.
	if err := copyFilesToGuest(ctx, tconn, filesApp, keyboard, testFiles, dirName); err != nil {
		return errors.Wrap(err, "failed to copy test files to guest")
	}

	// Check the file list in home directory is equal to the copied file list.
	fileList, err := guest.GetFileList(ctx, ".")
	if err != nil {
		return errors.Wrap(err, "failed to get files in home directory in container")
	}
	if !reflect.DeepEqual(testFiles, fileList) {
		return errors.Wrapf(err, "found unexpected files in guest; got %q, want %q", fileList, testFiles)
	}

	return nil
}

// copyFilesToGuest copies all files in Downloads to guest.
func copyFilesToGuest(ctx context.Context, tconn *chrome.TestConn, filesApp *filesapp.FilesApp, keyboard *input.KeyboardEventWriter, testFiles []string, dirName string) error {
	steps := []uiauto.Action{filesApp.OpenDownloads()}

	// Steps to wait all files to display.
	for _, file := range testFiles {
		steps = append(steps, filesApp.SelectFile(file))
	}

	copyMsg := nodewith.Name(fmt.Sprintf("Copying %d items to %s", len(testFiles), dirName)).Role(role.StaticText)
	ui := uiauto.New(tconn)
	steps = append(steps,
		// Select all files.
		keyboard.AccelAction("ctrl+A"),

		// Copy all files.
		keyboard.AccelAction("ctrl+C"),

		// Open shared guest directory to paste.
		filesApp.OpenDir(dirName, filesapp.FilesTitlePrefix+dirName),

		// Paste all files.
		keyboard.AccelAction("ctrl+V"))

	if err := uiauto.Combine("copy files from Downloads to guest", steps...)(ctx); err != nil {
		return err
	}

	if err := filesApp.WithTimeout(10 * time.Second).WaitUntilExists(copyMsg)(ctx); err != nil {
		testing.ContextLog(ctx, "Copying message was not found")
	}

	if err := ui.WithTimeout(time.Minute).WaitUntilGone(copyMsg)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy files to guest in 1 minute")
	}

	return nil
}
