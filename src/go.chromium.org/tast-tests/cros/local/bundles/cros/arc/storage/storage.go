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
	testAppName = "ARC File Editor Test"
	// Test app's package name.
	testAppPkgName = "org.chromium.arc.testapp.fileeditor"
	// Test app's APK file name.
	testAppApkName = "ArcFileEditorTest.apk"
	// IDs of UI elements shown on test app.
	fileContentID  = testAppPkgName + ":id/file_content"
	modifyButtonID = testAppPkgName + ":id/button_modify"

	// The message that should be added by ArcFileEditorTest when "Modify file" button is clicked.
	// This should be kept in sync with MainActivity.java of ArcFileEditorTest.
	messageAddedByApp = ", this is added by Android"
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
	// File content of the provided test file.
	FileContent string
	// Optional: If set to true, wait for file type to appear before opening the file.
	// Currently used by DriveFS to ensure metadata has arrived.
	CheckFileType bool
	// Optional: If set to true, skip checking if the test app can write to the test file opened
	// from Files app.
	ReadOnly bool
}

// TestFilesAppIntegration tests ARC storage integration with Files app for a test file in the
// specified directory, e.g. Google Drive, Downloads, MyFiles etc, using the test android app,
// ArcFileEditorTest. The tested scenario is as follows:
//  1. Open the file with the Android app via Files app's "Open with...", and validate the file
//     content read by the Android app (which is shown on its UI).
//  2. (optional, only when TestConfig.ReadOnly is false) Modify the file with the Android app and
//     validate the modification on the CrOS side with Files app's QuickView.
func TestFilesAppIntegration(ctx context.Context, a *arc.ARC, cr *chrome.Chrome, d *androidui.Device, config TestConfig) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if config.DirTitle == "" {
		config.DirTitle = filesapp.FilesTitlePrefix + config.DirName
	}

	testing.ContextLogf(ctx, "Performing TestFilesAppIntegration on: %s", config.DirName)

	testing.ContextLog(ctx, "Installing ArcFileEditorTest app")
	if err := a.Install(ctx, arc.APKPath(testAppApkName)); err != nil {
		return errors.Wrap(err, "failed to install ArcFileEditorTest app")
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

	testing.ContextLogf(ctx, "Testing opening %s with Android app", config.FileName)
	if err := openWithTestApp(ctx, files, config); err != nil {
		return errors.Wrap(err, "could not open file with ArcFileEditorTest")
	}
	defer a.Command(cleanupCtx, "am", "force-stop", testAppPkgName).Run(testexec.DumpLogOnError)

	if err := validateLabel(ctx, d, fileContentID, config.FileContent); err != nil {
		return errors.Wrap(err, "failed to validate file content")
	}

	if config.ReadOnly {
		return nil
	}
	testing.ContextLogf(ctx, "Testing writing to %s with Android app", config.FileName)
	if err := d.Object(androidui.ID(modifyButtonID)).Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click modify button")
	}
	if err := validateLabel(ctx, d, fileContentID, config.FileContent+messageAddedByApp); err != nil {
		return errors.Wrap(err, "failed to validate file content modified by app")
	}
	// Close Android app window to ensure that Files app window UI is visible in
	// |validateWriteResult|.
	if err := a.Command(ctx, "am", "force-stop", testAppPkgName).Run(testexec.DumpLogOnError); err != nil {
		// Do not mark the entire test as failure, since in ARCVM R+ Files app window will
		// automatically come in front of the Android app window in |validateWriteResult|.
		testing.ContextLog(ctx, "Failed to close test app window")
	}
	return validateWriteResult(ctx, files, config)
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

// openWithTestApp opens the test file with ArcFileEditorTest.
func openWithTestApp(ctx context.Context, files *filesapp.FilesApp, config TestConfig) error {
	testing.ContextLog(ctx, "Opening the test file with ArcFileEditorTest")

	return uiauto.Combine("open the test file with ArcFileEditorTest",
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

// validateLabel is a helper function to load app label texts and compare it with expectation.
func validateLabel(ctx context.Context, d *androidui.Device, labelID, expected string) error {
	testing.ContextLogf(ctx, "Validating label content of %s with %q", labelID, expected)
	return d.Object(androidui.ID(labelID)).WaitForText(ctx, expected, uiTimeout)
}

// validateWriteResult validates the modification to the file made with test Android app using
// Files app's QuickView.
func validateWriteResult(ctx context.Context, files *filesapp.FilesApp, config TestConfig) error {
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	return uiauto.Combine("validate the content of test file with QuickView",
		files.OpenPath(config.DirTitle, config.DirName, config.SubDirectories...),
		files.SelectFile(config.FileName),
		keyboard.AccelAction("Space"),
		files.WithTimeout(5*time.Second).WaitUntilExists(nodewith.Name(config.FileContent+messageAddedByApp).Role(role.StaticText)),
		keyboard.AccelAction("Space"),
	)(ctx)
}
