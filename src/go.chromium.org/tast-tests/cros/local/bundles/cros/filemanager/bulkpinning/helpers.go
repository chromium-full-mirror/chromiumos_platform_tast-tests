// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bulkpinning package contains helpers to "Everything offline" feature.
package bulkpinning

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SettingsToggleFinder is the toggle to enable / disable File sync from the
// Google Drive OS Settings page.
var SettingsToggleFinder = nodewith.Role(role.ToggleButton).Name("File sync")

// dialogConfirmButton is the standard confirmation button on all dialogs that
// appear in the Google Drive OS Settings pages.
var dialogConfirmButton = nodewith.Role(role.Button).ClassName("action-button")

// GoogleDriveSettingsPageURL is the subpage URL to navigate directly to the
// google drive page in OS settings.
const GoogleDriveSettingsPageURL = "googleDrive"

// CreateTestFile will create a test file via the Google Drive API then wait
// for it to be available locally on DriveFS.
func CreateTestFile(ctx context.Context, apiClient *drivefs.APIClient, driveFsClient *drivefs.DriveFs, fileDataPath, testFileName string) (string, error) {
	const (
		retryAttempts = 20
		retryInterval = 5 * time.Second
	)

	// Create the test file with the Drive API.
	driveFile, err := apiClient.CreateFileFromLocalFile(ctx,
		testFileName, "root", fileDataPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create test file: %q", testFileName)
	}
	testing.ContextLogf(ctx, "Created %s with ID: %s", testFileName, driveFile.Id)

	// Wait for file to be available locally.
	testFilePath := driveFsClient.MyDrivePath(testFileName)
	testFile, err := driveFsClient.NewFile(testFilePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to build drivefs file")
	}
	if err = action.RetrySilently(retryAttempts, testFile.ExistsAction(), retryInterval)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to wait for file to be available locally")
	}
	return driveFile.Id, nil
}

// toggleBulkPinningInSettings enables / disables bulk pinning from the Google
// Drive OS Settings page.
func toggleBulkPinningInSettings(ui *uiauto.Context, enabled bool) uiauto.Action {
	enableBulkPinningAction := uiauto.Combine("enable bulk pinning",
		// Ensure the toggle is disabled to begin with.
		ui.Exists(SettingsToggleFinder.Attribute("checked", "false")),
		// Enable the bulk pinning toggle in OS Settings.
		ui.LeftClick(SettingsToggleFinder),
		// Wait for it to move to checked state.
		ui.WaitUntilExists(SettingsToggleFinder.Attribute("checked", "true")),
	)

	disableBulkPinningAction := uiauto.Combine("disable bulk pinning",
		// Disable the bulk pinning toggle in OS Settings.
		ui.LeftClick(SettingsToggleFinder),
		// Wait for the confirmation dialog action button to appear.
		ui.WaitUntilExists(dialogConfirmButton),
		// Click and confirm the turn off confirmation dialog.
		ui.LeftClick(dialogConfirmButton),
		// Wait for the toggle button to enter the disabled state.
		ui.WaitUntilExists(SettingsToggleFinder.Attribute("checked", "false")),
	)

	if enabled {
		return enableBulkPinningAction
	}

	return disableBulkPinningAction
}

// EnableBulkPinningFromSettings opens the OS settings page to the /googleDrive
// subpage and turns on the bulk pinning feature.
func EnableBulkPinningFromSettings(cr *chrome.Chrome, tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, GoogleDriveSettingsPageURL, ui.Exists(SettingsToggleFinder)); err != nil {
			return errors.Wrap(err, "failed to launch Google Drive settings")
		}

		return toggleBulkPinningInSettings(ui, true)(ctx)
	}
}

// DisableBulkPinningFromSettings opens the OS settings page to the /googleDrive
// subpage and turns off the bulk pinning feature.
func DisableBulkPinningFromSettings(cr *chrome.Chrome, tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, GoogleDriveSettingsPageURL, ui.Exists(SettingsToggleFinder)); err != nil {
			return errors.Wrap(err, "failed to launch Google Drive settings")
		}

		return toggleBulkPinningInSettings(ui, false)(ctx)
	}
}

// CleanUpStorageFromSettings disables bulk pinning if it's enabled then
// proceeds to press and confirm the "Clean up storage" button from the Google
// Drive settings page.
func CleanUpStorageFromSettings(cr *chrome.Chrome, tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		cleanUpStorageButton := nodewith.Role(role.Button).Name("Clean up storage")
		if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, GoogleDriveSettingsPageURL, ui.Exists(cleanUpStorageButton)); err != nil {
			return errors.Wrap(err, "failed to launch Google Drive settings")
		}

		usingZeroBOrFourKBText := nodewith.Role(role.StaticText).NameRegex(regexp.MustCompile("Using (0 |4.0 K)B"))
		return uiauto.Combine("clean up storage in settings",
			// If bulk pinning is enabled, disable it first.
			uiauto.IfSuccessThen(
				ui.Exists(SettingsToggleFinder.Attribute("checked", "true")),
				toggleBulkPinningInSettings(ui, false),
			),
			// Keep clicking the "Clean up storage" button until the dialog confirm
			// button appears.
			ui.WithInterval(time.Second).LeftClickUntil(cleanUpStorageButton, ui.Exists(dialogConfirmButton)),
			// Click the dialog confirm button.
			ui.LeftClick(dialogConfirmButton),
			// Wait until the offline storage shows up as "Using 0 B" which happens
			// when all the files are removed or "Using 4.0 KB" which happens when a
			// file is pre-cached (via a read) and is not removed as part of the
			// "Clear offline files" button.
			ui.WaitUntilExists(usingZeroBOrFourKBText),
		)(ctx)
	}
}

// AssertFileAvailableOffline launches Files app and ensures the available
// offline tick is visible for the supplied `fileName`.
func AssertFileAvailableOffline(tconn *chrome.TestConn, fileName string, iconTimeout time.Duration) uiauto.Action {
	return func(ctx context.Context) error {
		files, err := filesapp.ExistingOrNew(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch Files app")
		}

		// NOTE: The accounts used are real accounts that are sometimes being used in
		// parallel with other tests, there may be other files in this list view, but
		// to avoid race conditions just check the file we created is available
		// offline.
		fileRow := nodewith.Role(role.ListBoxOption).NameContaining(fileName)
		availableOfflineIcon := nodewith.ClassNameRegex(regexp.MustCompile("(inline-status|tast-inline-status)")).Name("Available offline").Ancestor(fileRow)
		return uiauto.Combine("check file in drive has available offline toggle",
			// Open the Google Drive folder and check for the test file.
			files.OpenDrive(),
			// Wait for the file, if it can't find it try to maximize the window and find again.
			files.PerformActionAndRetryMaximizedOnFail(files.WaitForFile(fileName)),
			// Wait for the available offline icon to appear.
			files.WithTimeout(iconTimeout).WaitUntilExists(availableOfflineIcon),
		)(ctx)
	}
}
