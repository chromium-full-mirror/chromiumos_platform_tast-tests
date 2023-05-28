// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Timeout to wait for UI item to appear.
	uiTimeout = 10 * time.Second

	// Test app's name displayed in the context menu of the Files app.
	testAppName = "ARC File Reader Test"
	// Test app's package name.
	testAppPkgName = "org.chromium.arc.testapp.filereader"
	// Test app's APK file name.
	testAppApkName = "ArcFileReaderTest.apk"
)

// TestConfig stores the details of the directory under test and misc test configurations.
type TestConfig struct {
	// Name of the directory. This should be present on the sidebar of the Files app when it is
	// launched.
	DirName string
	// If specified, open the sub-directories under "DirName".
	SubDirectories []string
	// Name of the test file to be used in the test.
	FileName string
	// Optional: Expected title of the Ash window of the Files app when opened |DirName| on the
	// navigation tree. When unspecified, |filesapp.FilesTitlePrefix + DirName| will be used.
	DirTitle string
	// Expected file content of the test file.
	FileContent string
	// Optional: If set to true, wait for file type to appear before opening the file.
	// Currently used by DriveFS to ensure metadata has arrived.
	CheckFileType bool
}

// TestOpenWithAndroidApp opens a test file in the specified directory, e.g. Google Drive,
// Downloads, MyFiles etc, using the test android app, ArcFileReaderTest. The app will display
// the intent action, URI and file content on its UI, and the displayed file content is validated
// against the expected value.
func TestOpenWithAndroidApp(ctx context.Context, a *arc.ARC, cr *chrome.Chrome, d *androidui.Device, config TestConfig) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if config.DirTitle == "" {
		config.DirTitle = filesapp.FilesTitlePrefix + config.DirName
	}

	testing.ContextLogf(ctx, "Performing TestOpenWithAndroidApp on: %s", config.DirName)

	testing.ContextLog(ctx, "Installing ArcFileReaderTest app")
	if err := a.Install(ctx, arc.APKPath(testAppApkName)); err != nil {
		return errors.Wrap(err, "failed to install ArcFileReaderTest app")
	}

	if err := a.WaitIntentHelper(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for ARC Intent Helper")
	}

	files, err := openFilesApp(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "failed to open Files app")
	}
	defer files.Close(cleanupCtx)

	if config.CheckFileType {
		if err := waitForFileType(ctx, files, config); err != nil {
			return errors.Wrap(err, "failed to wait for file type to be populated")
		}
	}

	if err := openWithReaderApp(ctx, files, config); err != nil {
		return errors.Wrap(err, "could not open file with ArcFileReaderTest")
	}
	defer a.Command(cleanupCtx, "am", "force-stop", testAppPkgName).Run(testexec.DumpLogOnError)

	if err := validateResult(ctx, d, config); err != nil {
		return errors.Wrap(err, "ArcFileReaderTest's data is invalid")
	}
	return nil
}

// openFilesApp opens the Files App and returns a pointer to it.
func openFilesApp(ctx context.Context, cr *chrome.Chrome) (*filesapp.FilesApp, error) {
	testing.ContextLog(ctx, "Opening Files App")

	// Open the test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}

	// Open the Files App with default timeouts.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "launching the Files App failed")
	}

	return files, nil
}

// openWithReaderApp opens the test file with ArcFileReaderTest.
func openWithReaderApp(ctx context.Context, files *filesapp.FilesApp, config TestConfig) error {
	testing.ContextLog(ctx, "Opening the test file with ArcFileReaderTest")

	return uiauto.Combine("open the test file with ArcFileReaderTest",
		files.OpenPath(config.DirTitle, config.DirName, config.SubDirectories...),
		// Note: due to the banner loading, this may still be flaky.
		// If that is the case, we may want to increase the interval and timeout for this next call.
		files.SelectFile(config.FileName),
		files.ClickContextMenuItem(config.FileName, filesapp.OpenWith, testAppName),
	)(ctx)
}

// waitForFileType waits for file type (mime type) to be populated. This is an
// indication that the backend metadata is ready.
func waitForFileType(ctx context.Context, files *filesapp.FilesApp, config TestConfig) error {
	if err := uiauto.Combine("select the test file with Files app",
		files.OpenPath(config.DirTitle, config.DirName, config.SubDirectories...),
		files.SelectFile(config.FileName))(ctx); err != nil {
		return errors.Wrap(err, "failed to select the test file with Files app")
	}

	// Get the keyboard.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	// Press 'Space' to open/close the QuickView and check the file type. Repeat this
	// until 'text/plain' is shown. Retries up to 6 times (~ 30 seconds).
	times := 6
	if err := uiauto.Retry(times,
		uiauto.Combine("Checking file type",
			keyboard.AccelAction("Space"),
			files.WithTimeout(5*time.Second).WaitUntilExists(nodewith.Name("text/plain").Role(role.StaticText)),
			keyboard.AccelAction("Space"),
		),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to wait for file type after %d retries", times)
	}
	return nil
}

// validateResult validates the data read from ArcFileReaderTest app.
func validateResult(ctx context.Context, d *androidui.Device, config TestConfig) error {
	const fileContentID = testAppPkgName + ":id/file_content"

	testing.ContextLog(ctx, "Validating result in ArcFileReaderTest")

	return validateLabel(ctx, d, fileContentID, config.FileContent)
}

// validateLabel is a helper function to load app label texts and compare it with expectation.
func validateLabel(ctx context.Context, d *androidui.Device, labelID, expected string) error {
	uiObj := d.Object(androidui.ID(labelID))
	if err := uiObj.WaitForExists(ctx, uiTimeout); err != nil {
		return errors.Wrapf(err, "failed to find the label id %s", labelID)
	}

	actual, err := uiObj.GetText(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get text from the label id %s", labelID)
	}

	if actual != expected {
		return errors.Errorf("unexpected value in label %s: got %q, want %q", labelID, actual, expected)
	}

	testing.ContextLogf(ctx, "Label content of %s = %s", labelID, actual)
	return nil
}
