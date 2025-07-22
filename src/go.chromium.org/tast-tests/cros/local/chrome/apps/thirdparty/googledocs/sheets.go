// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googledocs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// sheetsName represents the name of the Google Sheets web area.
const sheetsName = "Google Sheets"

var (
	// SheetsWindow represents the window of the Google Sheets.
	SheetsWindow = nodewith.NameContaining(sheetsName).Role(role.Window).First()
	// SheetsWebArea represents the web area of the Google Sheets.
	SheetsWebArea = nodewith.NameContaining(sheetsName).Role(role.RootWebArea).First()
)

// DeleteSheetsWithURL returns an action to open the sheets url and delete the document.
func DeleteSheetsWithURL(tconn *chrome.TestConn, cr *chrome.Chrome, url, outDir string) action.Action {
	return func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", url)
		}
		defer conn.Close()
		defer conn.CloseTarget(cleanupCtx)
		// Dump the UI tree and the screenshot before closing the sheets page.
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, cr, "delete_sheets")

		if err := webutil.WaitForQuiescence(ctx, conn, pageLoadTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for the page to load")
		}
		return DeleteSheets(tconn)(ctx)
	}
}

// DeleteSheets returns an action to delete the document.
func DeleteSheets(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	application := nodewith.Role(role.Application).Ancestor(SheetsWebArea) // Google Sheets application node.
	fileButton := nodewith.Name("File").Role(role.MenuItem).Ancestor(application).Onscreen()
	menu := nodewith.Role(role.Menu).Ancestor(application)
	goToSheetsHome := nodewith.Name("Go to Sheets home screen").Role(role.Button)

	return uiauto.NamedCombine("delete document",
		cuj.ExpandMenu(tconn, fileButton, menu, 400),
		ui.DoDefault(moveToTrashItem),
		ui.DoDefault(goToSheetsHome),
		ui.WaitUntilGone(goToSheetsHome),
	)
}

// DeleteCellValue used for clean the content value in the cell.
func DeleteCellValue(ctx context.Context, tconn *chrome.TestConn) error {
	var webArea = nodewith.NameContaining("Sheets").Role(role.RootWebArea)
	ui := uiauto.New(tconn)
	application := nodewith.Role(role.Application).Ancestor(webArea)
	menu := nodewith.Role(role.Menu).Ancestor(application)
	editButton := nodewith.Name("Edit").Role(role.MenuItem).Ancestor(application).Onscreen()
	delete := nodewith.NameContaining("Delete").Role(role.MenuItem)
	values := nodewith.NameContaining("Values").Role(role.MenuItem)

	if err := uiauto.NamedCombine("delete cell value",
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(delete, ui.WithTimeout(5*time.Second).WaitUntilExists(values)),
		ui.DoDefaultUntil(values, ui.WithTimeout(5*time.Second).WaitUntilGone(values)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to delete cell value")
	}
	return nil
}

// EditSheet returns an action that edits google sheet.
func EditSheet(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, text string) action.Action {
	return uiauto.NamedCombine("edit sheet",
		kb.TypeAction(text),
		kb.AccelAction("Enter"),
		waitForSheetSaved(tconn),
	)
}

// waitForSheetSaved waits for the sheet document state to be saved.
func waitForSheetSaved(tconn *chrome.TestConn) action.Action {
	return waitForDocumentSaved(tconn, sheetsName)
}

// ChangeSheetFontSize returns an action to change sheet font size to specific font size.
func ChangeSheetFontSize(tconn *chrome.TestConn, size string) action.Action {
	ui := uiauto.New(tconn)
	moreButton := nodewith.Name("More").Role(role.ToggleButton).Ancestor(SheetsWebArea)
	fontSizeTextField := nodewith.Name("Font size").Role(role.TextField).Ancestor(SheetsWebArea)
	fontSizeOption := nodewith.Name(size).Role(role.ListBoxOption).Ancestor(SheetsWebArea)
	return uiauto.Retry(retryTimes, uiauto.NamedCombine("change sheet font size to "+size,
		uiauto.IfSuccessThen(ui.Gone(fontSizeTextField),
			ui.LeftClickUntil(moreButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(fontSizeTextField))),
		ui.LeftClickUntil(fontSizeTextField, ui.WithTimeout(shortUITimeout).WaitUntilExists(fontSizeOption)),
		ui.LeftClick(fontSizeOption),
		waitForDocsSaved(tconn),
	))
}

// ClickOnSheetsWebArea clicks on sheets's web area.
func ClickOnSheetsWebArea(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	return uiauto.Combine("click on sheet web area",
		ReloadSheetsIfLoadingIssueDialogAppears(ui),
		clickGotItIfExists(ui),
		ui.LeftClick(SheetsWebArea),
	)
}

// ClickOnSheetsCanvas clicks on sheets's canvas to focus on the sheet cell.
func ClickOnSheetsCanvas(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	sheetsCanvas := nodewith.Role(role.Canvas).Ancestor(SheetsWebArea)
	return uiauto.Combine("click on sheet canvas",
		ReloadSheetsIfLoadingIssueDialogAppears(ui),
		clickGotItIfExists(ui),
		ui.LeftClick(sheetsCanvas),
	)
}

// ShowTheSheetMenus shows the hidden Sheet menu.
func ShowTheSheetMenus(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	return uiauto.Combine("show the sheet menus",
		clickGotItIfExists(ui),
		uiauto.IfFailThen(ui.Exists(SheetsWebArea),
			ui.DoDefaultUntil(SheetsWindow,
				ui.WithTimeout(5*time.Second).WaitUntilExists(SheetsWebArea),
			),
		),
		showTheMenus(ui),
	)
}

// ReloadSheetsIfLoadingIssueDialogAppears reloads the sheet page if the
// "Loading issue" dialog appears.
func ReloadSheetsIfLoadingIssueDialogAppears(ui *uiauto.Context) action.Action {
	return reloadIfLoadingIssueDialogAppears(ui, SheetsWebArea)
}
