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
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CloudUpload represents an instance of the Cloud Upload UI.
type CloudUpload struct {
	ui       *uiauto.Context
	tconn    *chrome.TestConn
	provider filesconsts.Provider
}

// App returns an instance of the Cloud Upload that can be used to find/wait and
// interact with the CloudUpload WebUI, which is implemented here:
// https://source.chromium.org/chromium/chromium/src/+/main:chrome/browser/resources/chromeos/cloud_upload/
func App(tconn *chrome.TestConn, provider filesconsts.Provider) *CloudUpload {
	// Create a uiauto.Context with default timeout.
	ui := uiauto.New(tconn).WithInterval(500 * time.Millisecond)

	return &CloudUpload{ui: ui, tconn: tconn, provider: provider}
}

var fileHandlerDialog = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("Choose an app to open *"))
var fileHandlerOpenButton = nodewith.Ancestor(fileHandlerDialog).Role(role.Button).Name("Open")

// WaitFileHandlerAndChoose waits for the File Handlers dialog and chooses the target cloud to open office files.
func (cu *CloudUpload) WaitFileHandlerAndChoose() uiauto.Action {
	var option *nodewith.Finder
	if cu.provider == filesconsts.DriveFs {
		option = nodewith.Ancestor(fileHandlerDialog).Role(role.ListBoxOption).NameContaining("Google Drive")
	} else if cu.provider == filesconsts.OneDrive {
		option = nodewith.Ancestor(fileHandlerDialog).Role(role.ListBoxOption).NameContaining("Microsoft 365")
	}

	return uiauto.Combine("File handlers dialog: choose "+string(cu.provider),
		log("Starting File Handlers step"),
		cu.ui.WaitUntilExists(fileHandlerDialog),
		cu.ui.LeftClick(option),
		cu.ui.LeftClickUntil(fileHandlerOpenButton, cu.ui.Gone(fileHandlerDialog)),
	)
}

// WaitGetStartedDialogAndClickNext waits for the dialog Get Started and clicks the button to proceed.
func (cu *CloudUpload) WaitGetStartedDialogAndClickNext() uiauto.Action {
	dialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("Set up Microsoft 365 *"))
	getStartedButton := nodewith.Ancestor(dialog).Role(role.Button).Name("Get started")
	return uiauto.Combine("Setup OneDrive dialog: get started",
		log("Starting Get Started step"),
		cu.ui.WaitUntilExists(dialog),
		cu.ui.LeftClickUntil(getStartedButton, cu.ui.Gone(dialog)),
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
		cu.ui.LeftClickUntil(connectButton, cu.ui.Gone(connectToOneDriveDialog)),
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
		cu.ui.LeftClickUntil(doneButton, cu.ui.Gone(SetupCompleteDialog)),
	)
}

// ConfirmUploadDialogForOneDrive is a finder for the OneDrive confirmation dialog.
var ConfirmUploadDialogForOneDrive = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Move|Copy) .* to .* OneDrive .*"))

// ConfirmUploadDialogForGoogleDrive is a finder for the Google Drive confirmation dialog.
var ConfirmUploadDialogForGoogleDrive = nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("(Move|Copy) .* to Google Drive .*"))

// OneDriveConnectedDialog to find the OneDrive Connected dialog.
var OneDriveConnectedDialog = nodewith.Role(role.Dialog).Name("Microsoft OneDrive connected")

// WaitOneDriveConnectedDialogAndClickClose waits for the OneDrive Connected dialog and clicks Close.
func (cu *CloudUpload) WaitOneDriveConnectedDialogAndClickClose() uiauto.Action {
	closeButton := nodewith.Ancestor(OneDriveConnectedDialog).Role(role.Button).Name("Close")
	return uiauto.Combine("OneDrive Connected dialog: close",
		log("Starting OneDrive connected step"),
		cu.ui.WaitUntilExists(OneDriveConnectedDialog),
		cu.ui.LeftClickUntil(closeButton, cu.ui.Gone(OneDriveConnectedDialog)),
	)
}

// WaitUploadConfirmationDialogAndClickToUpload waits for the dialog confirming copy or move to the Cloud, and confirms the upload.
func (cu *CloudUpload) WaitUploadConfirmationDialogAndClickToUpload(alwaysMove bool) uiauto.Action {
	dialog := ConfirmUploadDialogForGoogleDrive
	if cu.provider == filesconsts.OneDrive {
		dialog = ConfirmUploadDialogForOneDrive
	}
	moveButton := nodewith.Ancestor(dialog).Role(role.Button).NameRegex(regexp.MustCompile("(Move|Copy) and open"))
	alwaysMoveCheckbox := nodewith.Ancestor(dialog).Role(role.CheckBox)

	longerUI := cu.ui.WithTimeout(40 * time.Second)

	return uiauto.Combine("Move/copy to cloud dialog: upload",
		log("Starting Move/Copy confirmation dialog step"),
		longerUI.WaitUntilExists(dialog),
		func(ctx context.Context) error {
			if alwaysMove {
				return longerUI.LeftClick(alwaysMoveCheckbox)(ctx)
			}
			// Default to no-op action.
			return nil
		},
		longerUI.LeftClickUntil(moveButton, longerUI.Gone(dialog)),
	)
}

// MaybeConfirmUploadOr365Window waits for either the confirmation dialog and confirms it;
// or waits for the Microsoft 365 window and close it.
// This should be used when the confirmation dialog isn't relevant for the test.
func (cu *CloudUpload) MaybeConfirmUploadOr365Window(tconn *chrome.TestConn, ms365App *ms365.Ms365, fileName string) uiauto.Action {
	return func(ctx context.Context) error {
		found, err := cu.ui.WithTimeout(2*time.Minute).FindAnyExists(ctx,
			ConfirmUploadDialogForOneDrive,
			ms365.Microsoft365WindowFinder(fileName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to find the next step after Install PWA dialog")
		}
		if found == ConfirmUploadDialogForOneDrive {
			return uiauto.Combine("Confirm upload and wait to open",
				cu.WaitUploadConfirmationDialogAndClickToUpload(false),
				ms365App.WaitForMicrosoft365WindowAndClose(tconn, fileName),
			)(ctx)
		}
		return nil
	}
}

// OneDriveSetupFlowOptions contains the options required for running OneDrive setup flow.
type OneDriveSetupFlowOptions struct {
	Ms365App          *ms365.Ms365
	PWAInstalled      bool
	OneDriveConnected bool
}

// RunOneDriveSetupFlow runs the step to test the setup flow for OneDrive.
func (cu *CloudUpload) RunOneDriveSetupFlow(options *OneDriveSetupFlowOptions) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "MS user:", options.Ms365App.UserName)

		if err := uiauto.Combine("Setup dialog steps",
			// Dialog setting up the File Handler, configuring the file type to open with Office 365.
			cu.WaitFileHandlerAndChoose(),
			// Fist setup dialog.
			cu.WaitGetStartedDialogAndClickNext(),
			func(ctx context.Context) error {
				// Skip PWA install step if it's already installed.
				if options.PWAInstalled {
					return log("Skipping install PWA step")(ctx)
				}
				// Otherwise expect PWA install screen.
				// This step is quite slow because it downloads from the internet.
				return cu.WaitInstallPWADialogAndClickInstall()(ctx)
			},
			func(ctx context.Context) error {
				// Skip Connect OneDrive step if it's already connected.
				if options.OneDriveConnected {
					return log("Skipping Connect to OneDrive step")(ctx)
				}
				// Otherwise expect Connect OneDrive screen.
				return uiauto.Combine("Connect to OneDrive",
					// Connect/mount the ODFS.
					cu.WaitConnectToOneDriveDialogAndClickConnect(),
					// Authenticate to OneDrive to mount ODFS.
					// Skip password screen if PWA is already installed.
					options.Ms365App.LoginToMicrosoft365(SetupCompleteDialog, options.PWAInstalled /*=skipPassword*/),
				)(ctx)
			},
			// Last step of the setup flow.
			cu.WaitSetupCompleteDialogAndClickDone(),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to complete the setup dialog steps")
		}
		return nil
	}
}

// RunGoogleDriveSetupFlow runs the step to test the setup flow for Google Drive.
func (cu *CloudUpload) RunGoogleDriveSetupFlow() uiauto.Action {
	return cu.WaitFileHandlerAndChoose()
}
