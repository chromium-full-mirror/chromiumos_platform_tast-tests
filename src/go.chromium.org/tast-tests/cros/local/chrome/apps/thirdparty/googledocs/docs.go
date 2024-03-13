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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// docsName represents the name of the Google Docs web area.
const docsName = "Google Docs"

var (
	docsWebArea     = nodewith.NameContaining(docsName).Role(role.RootWebArea)
	docsApplication = nodewith.Role(role.Application).Ancestor(docsWebArea)
)

// NewGoogleDocs returns an action to create a new Google document.
func NewGoogleDocs(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser, uiHandler cuj.UIActionHandler, newWindow bool) error {
	var opts []browser.CreateTargetOption
	if newWindow {
		opts = append(opts, browser.WithNewWindow())
	}
	testing.ContextLog(ctx, "Start to create Google document")
	// If there is an account sign-out issue when navigating to a Google Docs page,
	// it will continue to evaluate the JS expression in br.NewConn until
	// the test case timeout is exceeded.
	// Set a short timeout value to return errors earlier.
	newChromeTabCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	conn, err := uiHandler.NewChromeTab(newChromeTabCtx, br, cuj.NewGoogleDocsURL, newWindow)
	if err != nil {
		return errors.Wrap(err, "failed to open the Google document")
	}
	defer conn.Close()
	return webutil.WaitForQuiescence(ctx, conn, longUITimeout)
}

// RenameDoc returns an action to rename the document.
func RenameDoc(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, title string) action.Action {
	ui := uiauto.New(tconn)
	renameTextbox := nodewith.Name("Rename").ClassName("docs-title-input").Ancestor(docsWebArea).Editable().Focusable()
	return ui.Retry(5, uiauto.NamedCombine("rename document",
		ShowTheDocMenus(tconn, kb),
		ui.WaitUntilExists(docsWebArea),
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
	content := nodewith.Name("Document content").Role(role.TextField).Ancestor(docsWebArea).Editable().First()
	canvas := nodewith.Role(role.Canvas).Ancestor(docsWebArea).First()
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
	moreButton := nodewith.Name("More").Role(role.ToggleButton).Ancestor(docsWebArea)
	textColorButton := nodewith.Name("Text color").Role(role.PopUpButton).Ancestor(docsWebArea)
	colorButton := nodewith.Name(color).Role(role.Cell).Ancestor(docsWebArea)
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
	moreButton := nodewith.Name("More").Role(role.ToggleButton).Ancestor(docsWebArea)
	fontSizeTextField := nodewith.Name("Font size").Role(role.TextField).Ancestor(docsWebArea)
	fontSizeOption18 := nodewith.Name(size).Role(role.ListBoxOption).Ancestor(docsWebArea)
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
	undoButton := nodewith.NameContaining("Undo").Role(role.Button).Ancestor(docsWebArea)
	return uiauto.NamedCombine("undo document",
		ui.LeftClick(undoButton),
		waitForDocsSaved(tconn),
	)
}

// RedoDoc returns an action to redo document.
func RedoDoc(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	redoButton := nodewith.NameContaining("Redo").Role(role.Button).Ancestor(docsWebArea)
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
	moveToTrash := nodewith.NameContaining("Move to trash t").Role(role.MenuItem)
	goToDocsHome := nodewith.Name("Go to Docs home screen").Role(role.Button)
	return uiauto.NamedCombine("delete document",
		cuj.ExpandMenu(tconn, fileButton, menu, 392),
		ui.WaitUntilExists(moveToTrash),
		uiauto.IfSuccessThenWithLog(ui.Exists(moveToTrash.State(state.Focusable, true)),
			uiauto.NamedCombine("move to trash",
				ui.DoDefault(moveToTrash),
				ui.DoDefault(goToDocsHome),
				// When leaving the edit document, the popup "Leave site?" might appear.
				// Click the leave button if it exists.
				prompts.ClearPotentialPrompts(tconn, 5*time.Second, prompts.LeaveSitePrompt),
				ui.WithTimeout(longUITimeout).WaitUntilExists(docHomeWebArea),
			)),
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
	hideTheMenusButton := nodewith.Name("Hide the menus (Ctrl+Shift+F)").Role(role.Button).Ancestor(docsApplication)
	showTheMenusButton := nodewith.Name("Show the menus (Ctrl+Shift+F)").Role(role.Button).Ancestor(docsApplication)
	showTheMenus := uiauto.NamedCombine("show the menus",
		kb.AccelAction("Ctrl+Shift+F"),
		ui.WaitUntilExists(hideTheMenusButton),
		ui.WaitForLocation(hideTheMenusButton))
	return uiauto.Combine("show the doc menus",
		ui.WaitUntilAnyExists(hideTheMenusButton, showTheMenusButton),
		uiauto.IfSuccessThen(ui.Exists(showTheMenusButton), showTheMenus))
}
