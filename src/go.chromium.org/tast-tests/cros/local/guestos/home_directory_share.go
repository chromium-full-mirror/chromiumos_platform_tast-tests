// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// FilesAppToGuest tests that file operations in the Files app are reflected in the guest.
func FilesAppToGuest(ctx context.Context, filesApp *filesapp.FilesApp, keyboard *input.KeyboardEventWriter, guest vm.Guest, dirName string) error {
	const testName = "test_folder"
	const testRename = "another_test_folder"

	if err := uiauto.Combine("Test Files app to guest",
		// Open the right folder.
		filesApp.OpenDir(dirName, filesapp.FilesTitlePrefix+dirName),

		// Files app doesn't have a way to directly create a file, but
		// we can create a folder, which is just as good.
		filesApp.CreateFolder(keyboard, testName),
		WaitForFile(guest, ".", testName),

		// Rename the test folder.
		filesApp.RenameFile(keyboard, testName, testRename),
		WaitForFile(guest, ".", testRename),

		// Delete the test folder.
		filesApp.DeleteFileOrFolder(keyboard, testRename),
		WaitForFileGone(guest, ".", testRename),
	)(ctx); err != nil {
		return errors.Wrap(err, "operations in Files app not reflected in guest")
	}

	return nil
}

// GuestToFilesApp tests that file operations in the guest are reflected in the Files app.
func GuestToFilesApp(ctx context.Context, filesApp *filesapp.FilesApp, guest vm.Guest, dirName string) error {
	const testName = "test_file"
	const testRename = "another_test_file"

	if err := uiauto.Combine("Test guest to Files app",
		// Open the right folder.
		filesApp.OpenDir(dirName, filesapp.FilesTitlePrefix+dirName),

		// Create file in container.
		commandAction(guest, "touch", testName),
		filesApp.WaitForFile(testName),

		// Rename file in container.
		commandAction(guest, "mv", testName, testRename),
		filesApp.WaitForFile(testRename),

		// Delete file in container.
		commandAction(guest, "rm", testRename),
		filesApp.WaitUntilFileGone(testRename),
	)(ctx); err != nil {
		return errors.Wrap(err, "operations in guest not reflected in Files app")
	}

	return nil
}

func commandAction(guest vm.Guest, args ...string) uiauto.Action {
	return func(ctx context.Context) error {
		return guest.Command(ctx, args...).Run(testexec.DumpLogOnError)
	}
}
