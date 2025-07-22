// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package googledocs provides the control of Google apps, including Google docs and Google slides.
package googledocs

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// docsName represents the name of the Google Docs web area.
const docsName = "Google Docs"

var (
	// DocsWindow represents the window of the Google Docs.
	DocsWindow = nodewith.NameContaining(docsName).Role(role.Window).First()
	// DocsWebArea represents the web area of the Google Docs.
	DocsWebArea     = nodewith.NameContaining(docsName).Role(role.RootWebArea).First()
	docsApplication = nodewith.Role(role.Application).Ancestor(DocsWebArea)
	moveToTrashItem = nodewith.NameContaining("Move to trash t").Role(role.MenuItem).First()
	menuBar         = nodewith.Name("Menu bar").Role(role.Banner).Ancestor(docsApplication)
)

// NewGoogleDocs returns an action to create a new Google document.
func NewGoogleDocs(ctx context.Context, uiHandler cuj.UIActionHandler, newWindow bool) error {
	testing.ContextLog(ctx, "Start to create Google document")
	// If there is an account sign-out issue when navigating to a Google Docs page,
	// it will continue to evaluate the JS expression until the test case timeout is exceeded.
	// Set a short timeout value to return errors earlier.
	newChromeTabCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	conn, err := uiHandler.NewChromeTab(newChromeTabCtx, cuj.NewGoogleDocsURL, newWindow)
	if err != nil {
		return errors.Wrap(err, "failed to open the Google document")
	}
	defer conn.Close()
	return webutil.WaitForQuiescence(ctx, conn, longUITimeout)
}

// RenameDoc returns an action to rename the document.
func RenameDoc(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, title string) action.Action {
	ui := uiauto.New(tconn)
	renameTextbox := nodewith.Name("Rename").ClassName("docs-title-input").Ancestor(DocsWebArea).Editable().Focusable()
	return ui.Retry(5, uiauto.NamedCombine("rename document",
		ShowTheDocMenus(tconn, kb),
		ui.WaitUntilExists(DocsWebArea),
		ui.LeftClickUntil(renameTextbox, ui.WithTimeout(5*time.Second).WaitUntilExists(renameTextbox.State("focused", true))),
		kb.AccelAction("Ctrl+A"),
		kb.TypeAction(title),
		waitForFieldTextToBe(tconn, renameTextbox, title),
		kb.AccelAction("Enter"),
		waitForDocsSaved(tconn),
	))
}

// EditDoc returns an action to edit the document.
func EditDoc(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, paragraph string) action.Action {
	ui := uiauto.New(tconn)
	content := nodewith.Name("Document content").Role(role.TextField).Ancestor(DocsWebArea).Editable().First()
	canvas := nodewith.Role(role.Canvas).Ancestor(DocsWebArea).First()
	return uiauto.NamedCombine("edit document",
		ui.WaitUntilExists(content),
		ui.LeftClick(canvas),
		kb.TypeAction(paragraph),
		waitForDocsSaved(tconn),
	)
}

// ChangeDocTextColor returns an action to change text color to specific color.
func ChangeDocTextColor(tconn *chrome.TestConn, color string) action.Action {
	ui := uiauto.New(tconn)
	moreButton := nodewith.Name("More").Role(role.ToggleButton).Ancestor(DocsWebArea)
	textColorButton := nodewith.Name("Text color").Role(role.PopUpButton).Ancestor(DocsWebArea)
	colorButton := nodewith.Name(color).Role(role.Cell).Ancestor(DocsWebArea)
	return uiauto.Retry(retryTimes, uiauto.NamedCombine("change document text color to "+color,
		uiauto.IfSuccessThen(ui.Gone(textColorButton), ui.LeftClickUntil(moreButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(textColorButton))),
		ui.LeftClickUntil(textColorButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(colorButton)),
		ui.LeftClick(colorButton),
		waitForDocsSaved(tconn),
	))
}

// ChangeDocFontSize returns an action to change font size to specific font size.
func ChangeDocFontSize(tconn *chrome.TestConn, size string) action.Action {
	ui := uiauto.New(tconn)
	moreButton := nodewith.Name("More").Role(role.ToggleButton).Ancestor(DocsWebArea)
	fontSizeTextField := nodewith.Name("Font size").Role(role.TextField).Ancestor(DocsWebArea)
	fontSizeOption18 := nodewith.Name(size).Role(role.ListBoxOption).Ancestor(DocsWebArea)
	return uiauto.Retry(retryTimes, uiauto.NamedCombine("change document font size to "+size,
		uiauto.IfSuccessThen(ui.Gone(fontSizeTextField), ui.LeftClickUntil(moreButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(fontSizeTextField))),
		ui.LeftClickUntil(fontSizeTextField, ui.WithTimeout(shortUITimeout).WaitUntilExists(fontSizeOption18)),
		ui.LeftClick(fontSizeOption18),
		waitForDocsSaved(tconn),
	))
}

// UndoDoc returns an action to undo document.
func UndoDoc(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	undoButton := nodewith.NameContaining("Undo").Role(role.Button).Ancestor(DocsWebArea)
	return uiauto.NamedCombine("undo document",
		ui.LeftClick(undoButton),
		waitForDocsSaved(tconn),
	)
}

// RedoDoc returns an action to redo document.
func RedoDoc(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	redoButton := nodewith.NameContaining("Redo").Role(role.Button).Ancestor(DocsWebArea)
	return uiauto.NamedCombine("redo document",
		ui.LeftClick(redoButton),
		waitForDocsSaved(tconn),
	)
}

// DeleteDoc returns an action to delete the document.
func DeleteDoc(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	docHomeWebArea := nodewith.Name(docsName).Role(role.RootWebArea).First()
	fileButton := nodewith.Name("File").Role(role.MenuItem).Ancestor(docsApplication)
	menu := nodewith.Role(role.Menu).Ancestor(docsApplication)
	goToDocsHome := nodewith.Name("Go to Docs home screen").Role(role.Button)
	moveToTrash := uiauto.NamedCombine("move to trash",
		ui.DoDefault(moveToTrashItem),
		ui.DoDefault(goToDocsHome),
		// When leaving the edit document, the popup "Leave site?" might appear.
		// Click the leave button if it exists.
		prompts.ClearPotentialPrompts(tconn, 5*time.Second, prompts.LeaveSitePrompt),
		ui.WithTimeout(longUITimeout).WaitUntilExists(docHomeWebArea),
	)
	return uiauto.NamedCombine("delete document",
		cuj.ExpandMenu(tconn, fileButton, menu, 392),
		ui.WaitUntilExists(moveToTrashItem),
		// If the document has not been modified, the menu item "Move to Trash"
		// will be disabled, and there is no need to delete the file because it
		// has not been successfully created.
		// Check if the menu item "Move to Trash" is disabled.
		// If it's not disabled, proceed to perform the move-to-trash action.
		uiauto.IfSuccessThenWithLog(
			ui.Gone(moveToTrashItem.HasClass("goog-menuitem-disabled")),
			moveToTrash,
		),
	)
}

// DeleteDocWithURL returns an action to open the doc url and delete the document.
func DeleteDocWithURL(tconn *chrome.TestConn, cr *chrome.Chrome, url string) action.Action {
	return func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", url)
		}
		defer conn.Close()
		defer conn.CloseTarget(cleanupCtx)

		if err := webutil.WaitForQuiescence(ctx, conn, pageLoadTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for the page to load")
		}
		return DeleteDoc(tconn)(ctx)
	}
}

// waitForDocsSaved waits for the docs document state to be saved.
func waitForDocsSaved(tconn *chrome.TestConn) action.Action {
	return waitForDocumentSaved(tconn, docsName)
}

// WaitUntilDocContentToBe waits for up to 5s until expected content.
func WaitUntilDocContentToBe(tconn *chrome.TestConn, expectedContent string) action.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			docContent, err := docContent(ctx, tconn)
			if err != nil {
				return err
			}
			if docContent != expectedContent {
				return errors.Errorf("unexpected gdoc content: got %q; want %q", docContent, expectedContent)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second})
	}
}

func docContent(ctx context.Context, tconn *chrome.TestConn) (string, error) {
	ui := uiauto.New(tconn)
	menu := nodewith.Role(role.Menu).Ancestor(docsApplication)
	editButton := nodewith.Name("Edit").Role(role.MenuItem).Ancestor(docsApplication)
	selectAllMenuItem := nodewith.NameContaining("Select all").Role(role.MenuItem)
	copyMenuItem := nodewith.NameContaining("Copy").Role(role.MenuItem)

	if err := uiauto.NamedCombine("copy document content",
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(selectAllMenuItem, ui.WithTimeout(shortUITimeout).WaitUntilGone(selectAllMenuItem)),
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(copyMenuItem, ui.WithTimeout(shortUITimeout).WaitUntilGone(copyMenuItem)),
		uiauto.Sleep(100*time.Millisecond), // Wait for clipboard set.
	)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to copy doc content to clipboard")
	}

	// Gdoc "Select all" automatically adds a new line in the end.
	// It needs to be trimmed from clipboard.
	var clipData string
	if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.getClipboardTextData)()`, &clipData); err != nil {
		return "", errors.Wrap(err, "failed to get clipboard content")
	}
	return strings.TrimRight(clipData, "\n"), nil
}

// ShowTheDocMenus shows the doc menus if it's hidden.
func ShowTheDocMenus(tconn *chrome.TestConn, kb *input.KeyboardEventWriter) action.Action {
	ui := uiauto.New(tconn)
	modeAndViewToolBar := nodewith.Name("Mode and view").Role(role.Toolbar).Ancestor(docsApplication)
	exitFullScreen := uiauto.NamedCombine("exit full screen",
		kb.AccelAction("Esc"),
		ui.WaitUntilExists(modeAndViewToolBar),
	)
	return uiauto.Retry(3,
		uiauto.Combine("show the doc menus",
			clickGotItIfExists(ui),
			uiauto.IfFailThen(ui.Exists(DocsWebArea),
				ui.DoDefaultUntil(DocsWindow,
					ui.WithTimeout(5*time.Second).WaitUntilExists(DocsWebArea),
				),
			),
			ui.WaitUntilExists(menuBar),
			reloadDocsIfLoadingIssueDialogAppears(ui),
			reloadIfFileMenuDisabled(ui),
			// In some cases, the toolbar is hidden in full screen.
			uiauto.IfFailThen(ui.Exists(modeAndViewToolBar), exitFullScreen),
			showTheMenus(ui)))
}

// reloadDocsIfLoadingIssueDialogAppears reloads the doc page if the
// "Loading issue" dialog appears.
func reloadDocsIfLoadingIssueDialogAppears(ui *uiauto.Context) action.Action {
	return reloadIfLoadingIssueDialogAppears(ui, DocsWebArea)
}

// reloadIfFileMenuDisabled reloads the page if the "File" menu is disabled
// for more than a minute.
func reloadIfFileMenuDisabled(ui *uiauto.Context) action.Action {
	fileMenuItem := nodewith.Name("File").Role(role.MenuItem).Ancestor(DocsWindow)
	fileMenuItemDisabled := fileMenuItem.HasClass("goog-control-disabled")
	reloadButton := nodewith.Name("Reload").Role(role.Button).Ancestor(DocsWindow)
	reloadPage := uiauto.NamedCombine("reload page because File menu is disabled",
		ui.DoDefault(reloadButton),
		ui.WaitUntilGone(menuBar),
		ui.WithTimeout(time.Minute).WaitUntilExists(menuBar),
	)
	return uiauto.Combine("reload if file menu disabled",
		ui.WaitUntilExists(fileMenuItem),
		uiauto.IfFailThen(ui.WithTimeout(time.Minute).WaitUntilGone(fileMenuItemDisabled), reloadPage),
	)
}
