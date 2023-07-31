// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package printmanagementapp contains common functions used in the app.
package printmanagementapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// PrintManagementApp represents an instance of the Print Management app.
type PrintManagementApp struct {
	ui    *uiauto.Context
	tconn *chrome.TestConn
}

// printManagementHistoryLabel is used to find the label for the History
// section.
var printManagementHistoryLabel = nodewith.Name("History").Role(role.StaticText)

// printManagementPrintJobEntry is used to find all print job entries.
var printManagementPrintJobEntry = nodewith.HasClass("list-item flex-center")

// PrintManagementDeleteHistoryButton is used to delete printing history.
var PrintManagementDeleteHistoryButton = nodewith.Name("Clear all history").Role(role.Button)

// printManagementDeleteConfirmButton is used to confirm deleting printing
// history.
var printManagementDeleteConfirmButton = nodewith.Name("Clear").HasClass("action-button").Role(role.Button)

// printManagementLaunchSettingsButton is used to launch Printer settings from
// Print Management SWA.
var printManagementLaunchSettingsButton = nodewith.Name("Manage printers").Role(role.Button)

// printManagementWindow is the main window for the print management dialog.
var printManagementWindow = nodewith.Name("Print jobs").Role(role.Window).First()

// Launch Print Management app via default method.
func Launch(ctx context.Context, tconn *chrome.TestConn) (*PrintManagementApp, error) {
	if err := apps.Launch(ctx, tconn, apps.PrintManagement.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch Print Management app")
	}

	if err := ash.WaitForApp(ctx, tconn, apps.PrintManagement.ID, time.Minute); err != nil {
		return nil, errors.Wrap(err, "Print Management app did not appear in shelf after launch")
	}

	return &PrintManagementApp{ui: uiauto.New(tconn), tconn: tconn}, nil
}

// ClearHistory returns an action that clears the print job history.
func (p *PrintManagementApp) ClearHistory() uiauto.Action {
	return uiauto.Combine("clear print job history",
		p.ui.WithTimeout(5*time.Second).WaitUntilExists(PrintManagementDeleteHistoryButton),
		// There may not be any jobs in the history, in which case the confirm
		// dialog won't appear.  Only try and click it if it appears.
		uiauto.IfSuccessThen(p.VerifyHistoryLabel(),
			uiauto.Combine("clear print job history and confirm",
				p.ui.DoDefault(PrintManagementDeleteHistoryButton),
				p.ui.WithTimeout(5*time.Second).WaitUntilExists(printManagementDeleteConfirmButton),
				p.ui.DoDefault(printManagementDeleteConfirmButton),
				p.ui.WaitUntilGone(printManagementPrintJobEntry))),
	)
}

// Focus returns an action that ensures the app window has focus.
func (p *PrintManagementApp) Focus() uiauto.Action {
	return p.ui.EnsureFocused(printManagementWindow)
}

// LaunchPrinterSettings returns an action that opens Printer settings.
func (p *PrintManagementApp) LaunchPrinterSettings() uiauto.Action {
	printerSettingsWindow := nodewith.Name("Settings - Printers").Role(role.Window).First()
	return uiauto.IfSuccessThen(p.verifyLaunchSettingsButton(),
		uiauto.Combine("press manage printers and confirm settings opens",
			p.ui.DoDefault(printManagementLaunchSettingsButton),
			p.ui.WaitUntilExists(printerSettingsWindow)))
}

// VerifyHistoryLabel returns an action that verifies the History section of the
// Print Management app is visible.
func (p *PrintManagementApp) VerifyHistoryLabel() uiauto.Action {
	return p.ui.WithTimeout(20 * time.Second).WaitUntilExists(printManagementHistoryLabel)
}

// VerifyPrintJob returns an action that verifies at least one print job is
// visible in the Print Management app.
func (p *PrintManagementApp) VerifyPrintJob() uiauto.Action {
	return p.ui.WithTimeout(20 * time.Second).WaitUntilExists(printManagementPrintJobEntry)
}

// verifyLaunchSettingsButton returns an action that verifies the
// button to launch Printer settings is visible.
func (p *PrintManagementApp) verifyLaunchSettingsButton() uiauto.Action {
	return p.ui.WithTimeout(5 * time.Second).WaitUntilExists(printManagementLaunchSettingsButton)
}
