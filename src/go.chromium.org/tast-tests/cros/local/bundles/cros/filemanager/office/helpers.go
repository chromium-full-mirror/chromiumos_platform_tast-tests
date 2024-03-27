// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package office package contains helpers to "Office" feature.
package office

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filesconsts"

	"go.chromium.org/tast/core/errors"
)

// OneDriveSettingsPageURL is the subpage URL to navigate directly to the
// office files page in OS settings.
const OneDriveSettingsPageURL = "oneDrive"

// OfficeFilesSettingsPageURL is the subpage URL to navigate directly to the
// office files page in OS settings.
const OfficeFilesSettingsPageURL = "officeFiles"

// SettingsOneDriveTitleFinder is the title of the Office files Settings subpage.
var SettingsOneDriveTitleFinder = nodewith.Role(role.Heading).Name("OneDrive")

// SettingsOfficeFilesTitleFinder is the title of the Office files Settings subpage.
var SettingsOfficeFilesTitleFinder = nodewith.Role(role.Heading).Name("Microsoft 365 files")

// LaunchOneDriveSettingsPage opens the OS settings page to the /oneDrive
// subpage.
func LaunchOneDriveSettingsPage(cr *chrome.Chrome, tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, OneDriveSettingsPageURL, ui.Exists(SettingsOneDriveTitleFinder)); err != nil {
			return errors.Wrap(err, "failed to launch OneDrive settings")
		}
		return nil
	}
}

// LaunchOfficeFilesSettingsPage opens the OS settings page to the /officeFiles
// subpage.
func LaunchOfficeFilesSettingsPage(cr *chrome.Chrome, tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, OfficeFilesSettingsPageURL, ui.Exists(SettingsOfficeFilesTitleFinder)); err != nil {
			return errors.Wrap(err, "failed to launch Office files settings")
		}
		return nil
	}
}

// ConnectToOneDrive logs into OneDrive through Settings and makes sure the ODFS shows in the Files's directory tree.
func ConnectToOneDrive(cr *chrome.Chrome, tconn *chrome.TestConn, files *filesapp.FilesApp, ms365App *ms365.Ms365) uiauto.Action {
	settingsApp := ossettings.New(tconn)
	cloudUpload := cloudupload.App(tconn, filesconsts.OneDrive)
	connectAccountButton := nodewith.Name("Connect").Role(role.Button)
	return uiauto.Combine("Connect to OneDrive via the OneDrive settings page",
		LaunchOneDriveSettingsPage(cr, tconn),
		settingsApp.LeftClick(connectAccountButton),
		cloudUpload.WaitConnectToOneDriveDialogAndClick(cloudupload.Next),
		ms365App.LoginToMicrosoft365(cloudupload.OneDriveConnectedDialog, false /*=skipPassword*/),
		cloudUpload.WaitOneDriveConnectedDialogAndClickClose(),
		settingsApp.Close,
		files.WaitUntilExists(nodewith.Name(filesapp.OneDrive).Role(role.TreeItem)),
	)
}
