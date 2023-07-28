// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cloudupload contains the page object and common functions for
// the cloud upload dialogs inside Files app.
package cloudupload

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CloudUpload represents an instance of the Cloud Upload UI.
type CloudUpload struct {
	ui    *uiauto.Context
	tconn *chrome.TestConn
}

// App returns an instance of the Cloud Upload that can be used to find/wait and
// interact with the CloudUpload WebUI, which is implemented here:
// https://source.chromium.org/chromium/chromium/src/+/main:chrome/browser/resources/chromeos/cloud_upload/
func App(tconn *chrome.TestConn) *CloudUpload {
	// Create a uiauto.Context with default timeout.
	ui := uiauto.New(tconn).WithInterval(500 * time.Millisecond)

	return &CloudUpload{ui: ui, tconn: tconn}
}

var fileHandlerDialog = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("Choose an app to open *"))
var fileHandlerOpenButton = nodewith.Ancestor(fileHandlerDialog).Role(role.Button).Name("Open")

// WaitFileHandlerAndChoose waits for the File Handlers dialog and chooses the target cloud to open office files.
func (cu *CloudUpload) WaitFileHandlerAndChoose(provider onedrive.Provider) uiauto.Action {

	var option *nodewith.Finder
	if provider == onedrive.DriveFs {
		option = nodewith.Ancestor(fileHandlerDialog).Role(role.ListBoxOption).NameRegex(regexp.MustCompile("Google * Uses Google Drive*"))
	} else if provider == onedrive.OneDrive {
		option = nodewith.Ancestor(fileHandlerDialog).Role(role.ListBoxOption).NameRegex(regexp.MustCompile("Microsoft 365*"))
	}

	return uiauto.Combine("File handlers dialog: choose Google Drive",
		log("Starting File Handlers step"),
		cu.ui.WaitUntilExists(fileHandlerDialog),
		cu.ui.LeftClick(option),
		cu.ui.LeftClick(fileHandlerOpenButton),
		cu.ui.WaitUntilGone(fileHandlerDialog),
	)
}

// WaitGetStartedDialogAndClickNext waits for the dialog Get Started and clicks the button to proceed.
func (cu *CloudUpload) WaitGetStartedDialogAndClickNext() uiauto.Action {
	dialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("Set up Microsoft 365 *"))
	getStartedButton := nodewith.Ancestor(dialog).Role(role.Button).Name("Get started")
	return uiauto.Combine("Setup OneDrive dialog: get started",
		log("Starting Get Started step"),
		cu.ui.WaitUntilExists(dialog),
		cu.ui.LeftClick(getStartedButton),
		cu.ui.WaitUntilGone(dialog),
	)
}

func log(msg string) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, msg)
		return nil
	}
}

// WaitInstallPWADialogAndClickInstall waits for the Install PWA dialog and clicks "Install".
func (cu *CloudUpload) WaitInstallPWADialogAndClickInstall() uiauto.Action {
	installPWADialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile(`Install.* Microsoft 365`))
	installButton := nodewith.Ancestor(installPWADialog).Role(role.Button).Name("Install")
	return uiauto.Combine("Install PWA dialog: install",
		log("Starting Install PWA step"),
		cu.ui.WaitUntilExists(installPWADialog),
		cu.ui.WaitUntilExists(installButton),
		cu.ui.LeftClick(installButton),
		cu.ui.WaitUntilGone(installButton),
		func(ctx context.Context) error {
			found, err := cu.ui.WithTimeout(2*time.Minute).FindAnyExists(ctx,
				installButton, // it will appear again if it fails to install.

				// The next page is either one of those dialogs.
				connectToOneDriveDialog,
				SetupCompleteDialog,
			)
			if err != nil {
				return errors.Wrap(err, "failed to find the next step after Install PWA dialog")
			}
			if found == installButton {
				if err := cu.WaitInstallPWADialogAndClickInstall()(ctx); err != nil {
					return err
				}
			}
			// The next dialogs were found, nothing to do.
			return nil
		},
	)
}

var connectToOneDriveDialog = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile(`Connect to Microsoft OneDrive.*`))

// WaitConnectToOneDriveDialogAndClickConnect waits for the Connect To OneDrive dialog and clicks "Connect to OneDrive".
func (cu *CloudUpload) WaitConnectToOneDriveDialogAndClickConnect() uiauto.Action {
	connectToOneDriveDialog := connectToOneDriveDialog
	connectButton := nodewith.Ancestor(connectToOneDriveDialog).Role(role.Button).Name("Connect to OneDrive")
	return uiauto.Combine("Connect to OneDrive dialog: connect",
		log("Starting Connect to OneDrive step"),
		cu.ui.WaitUntilExists(connectToOneDriveDialog),
		cu.ui.LeftClick(connectButton),
		cu.ui.WaitUntilGone(connectToOneDriveDialog),
	)
}

// SetupCompleteDialog to find the setup complete dialog.
var SetupCompleteDialog = nodewith.Role(role.Dialog).Name("Microsoft 365 setup complete")

// WaitSetupCompleteDialogAndClickDone waits for the Setup Complete dialog and clicks Done.
func (cu *CloudUpload) WaitSetupCompleteDialogAndClickDone() uiauto.Action {
	doneButton := nodewith.Ancestor(SetupCompleteDialog).Role(role.Button).Name("Done")
	return uiauto.Combine("Setup Complete dialog: done",
		log("Starting Setup Complete step"),
		cu.ui.WaitUntilExists(SetupCompleteDialog),
		cu.ui.LeftClick(doneButton),
		cu.ui.WaitUntilGone(SetupCompleteDialog),
	)
}

// ConfirmUploadDialog is a finder for the confirmation dialog.
var ConfirmUploadDialog = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Move|Copy) .* to .* OneDrive .*"))

// WaitUploadConfirmationDialogAndClickToUpload waits for the dialog confirming copy or move to the Cloud, and confirms the upload.
func (cu *CloudUpload) WaitUploadConfirmationDialogAndClickToUpload(alwaysMove bool) uiauto.Action {
	dialog := ConfirmUploadDialog
	moveButton := nodewith.Ancestor(dialog).Role(role.Button).NameRegex(regexp.MustCompile("(Move|Copy) and open"))
	alwaysMoveCheckbox := nodewith.Ancestor(dialog).Role(role.CheckBox)

	return uiauto.Combine("Move/copy to cloud dialog: done",
		log("Starting Move/Copy confirmation dialog step"),
		cu.ui.WaitUntilExists(dialog),
		func(ctx context.Context) error {
			if alwaysMove {
				return cu.ui.LeftClick(alwaysMoveCheckbox)(ctx)
			}
			// Default to no-op action.
			return nil
		},
		cu.ui.LeftClick(moveButton),
		cu.ui.WaitUntilGone(dialog),
	)
}

// MaybeConfirmUploadOr365Window waits for either the confirmation dialog and confirms it;
// or waits for the Microsoft 365 window.
// This should be used when the confirmation dialog isn't relevant for the test.
func (cu *CloudUpload) MaybeConfirmUploadOr365Window(ms365App *ms365.Ms365, fileName string) uiauto.Action {
	return func(ctx context.Context) error {
		found, err := cu.ui.WithTimeout(2*time.Minute).FindAnyExists(ctx,
			ConfirmUploadDialog,
			ms365.Microsoft365WindowFinder(fileName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to find the next step after Install PWA dialog")
		}
		if found == ConfirmUploadDialog {
			return uiauto.Combine("Confirm upload and wait to open",
				cu.WaitUploadConfirmationDialogAndClickToUpload(false),
				ms365App.WaitForMicrosoft365Window(fileName),
			)(ctx)
		}
		return nil
	}
}

// OneDriveSetupFlowOptions contains the options required for running OneDrive setup flow.
type OneDriveSetupFlowOptions struct {
	CloudUpload  *CloudUpload
	Ms365App     *ms365.Ms365
	PWAInstalled bool
}

// RunOneDriveSetupFlow runs the step to test the setup flow.
func RunOneDriveSetupFlow(options *OneDriveSetupFlowOptions) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "MS user:", options.Ms365App.UserName)

		if err := uiauto.Combine("Setup dialog steps",
			// Dialog setting up the File Handler, configuring the file type to open with Office 365.
			options.CloudUpload.WaitFileHandlerAndChoose(onedrive.OneDrive),
			// Fist setup dialog.
			options.CloudUpload.WaitGetStartedDialogAndClickNext(),
			func(ctx context.Context) error {
				// Skip PWA install step if it's already installed.
				if options.PWAInstalled {
					return log("Skipping install PWA step")(ctx)
				}
				// Otherwise expect PWA install screen.
				// This step is quite slow because it downloads from the internet.
				return options.CloudUpload.WaitInstallPWADialogAndClickInstall()(ctx)
			},
			// Connect/mount the ODFS.
			options.CloudUpload.WaitConnectToOneDriveDialogAndClickConnect(),
			// Authenticate to OneDrive to mount ODFS.
			// Skip password screen if PWA is already installed.
			options.Ms365App.LoginToMicrosoft365(SetupCompleteDialog, options.PWAInstalled /*=skipPassword*/),
			// Last step of the setup flow.
			options.CloudUpload.WaitSetupCompleteDialogAndClickDone(),
		)(ctx); err != nil {
			errors.Wrap(err, "failed to complete the setup dialog steps")
		}
		return nil
	}
}
