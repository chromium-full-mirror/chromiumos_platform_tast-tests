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

	// On first access, GuestOS/Crostini entries in the Files app navigation tree
	// are read-only placeholders whose window title updates immediately while
	// sshfs mounts asynchronously in the background. Open the guest directory
	// and wait for a sentinel file to appear so Files app has switched to the
	// real writable volume before attempting to paste.
	const mountSentinel = "mount_ready.txt"
	if err := guest.WriteFile(ctx, mountSentinel, "ready"); err != nil {
		return errors.Wrap(err, "failed to create mount sentinel file in guest")
	}
	if err := uiauto.Combine("wait for guest volume to mount in Files app",
		filesApp.OpenDir(dirName, filesapp.FilesTitlePrefix+dirName),
		filesApp.WaitForFile(mountSentinel),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to mount %q in Files app", dirName)
	}
	if err := guest.Cleanup(ctx, "."); err != nil {
		return errors.Wrap(err, "failed to cleanup mount sentinel file in guest")
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
		return errors.Errorf("found unexpected files in guest; got %q, want %q", fileList, testFiles)
	}

	return nil
}

// copyFilesToGuest copies all files in Downloads to guest.
func copyFilesToGuest(ctx context.Context, tconn *chrome.TestConn, filesApp *filesapp.FilesApp, keyboard *input.KeyboardEventWriter, testFiles []string, dirName string) error {
	steps := []uiauto.Action{filesApp.OpenDownloads()}

	// Wait for all files to display, then select one to focus the file list.
	for _, file := range testFiles {
		steps = append(steps, filesApp.WaitForFile(file))
	}
	steps = append(steps, filesApp.SelectFile(testFiles[0]))

	allSelectedSteps := make([]uiauto.Action, len(testFiles))
	for i, file := range testFiles {
		allSelectedSteps[i] = filesApp.IsFileSelected(file)
	}
	allSelected := uiauto.Combine("verify all test files selected", allSelectedSteps...)

	copyMsg := nodewith.Name(fmt.Sprintf("Copying %d items to %s", len(testFiles), dirName)).Role(role.StaticText)
	copyMenuItem := nodewith.Name(filesapp.Copy).Role(role.MenuItem)
	firstFile := nodewith.Name(testFiles[0]).Role(role.StaticText).Ancestor(nodewith.Role(role.ListBox))
	ui := uiauto.New(tconn)
	steps = append(steps,
		// Select all files and verify selection before copying.
		filesApp.RetryUntil(keyboard.AccelAction("ctrl+A"), allSelected),

		// Copy via context menu and wait for it to close so the copy handler
		// completes before navigating away from Downloads.
		filesApp.RightClickUntil(firstFile, filesApp.Exists(copyMenuItem)),
		filesApp.LeftClick(copyMenuItem),
		filesApp.WaitUntilGone(copyMenuItem),

		// Open shared guest directory, wait for stale Downloads entries to clear
		// from the accessibility tree, and focus the file list before pasting.
		filesApp.OpenDir(dirName, filesapp.FilesTitlePrefix+dirName))

	for _, file := range testFiles {
		steps = append(steps, filesApp.WaitUntilFileGone(file))
	}

	steps = append(steps,
		filesApp.EnsureFocused(nodewith.Role(role.ListBox)),

		// Paste all files.
		keyboard.AccelAction("ctrl+V"))

	if err := uiauto.Combine("copy files from Downloads to guest", steps...)(ctx); err != nil {
		return err
	}

	// Wait for all copied files to appear in Files app and for any copy
	// progress banner to finish before inspecting the guest filesystem.
	for _, file := range testFiles {
		if err := filesApp.WithTimeout(time.Minute).WaitForFile(file)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find copied file %q in %q", file, dirName)
		}
	}

	if err := ui.WithTimeout(time.Minute).WaitUntilGone(copyMsg)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy files to guest in 1 minute")
	}

	return nil
}
