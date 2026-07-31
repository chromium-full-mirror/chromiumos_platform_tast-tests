// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googledocs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	longUITimeout      = time.Minute      // Used for situations where UI elements that need more time to appear.
	pageLoadTimeout    = 30 * time.Second // Used for waiting for page to load.
	shortUITimeout     = 5 * time.Second  // Used for situations where UI response are faster.
	saveToDriveTimeout = 5 * time.Second  // saveToDriveTimeout indicates the maximum waiting time for Google to save to Drive.
	retryTimes         = 3                // Used for some operations that need to be retried.
)

// waitForDocumentSaved waits for the document state to "Saved to Drive".
func waitForDocumentSaved(tconn *chrome.TestConn, appName string) action.Action {
	ui := uiauto.New(tconn)
	webArea := nodewith.NameContaining(appName).Role(role.RootWebArea)
	documentSavedState := nodewith.NameContaining("Document status: Saved to Drive").Role(role.Button).Ancestor(webArea)
	return func(ctx context.Context) error {
		startTime := time.Now()
		if err := ui.WithTimeout(saveToDriveTimeout).WaitUntilExists(documentSavedState)(ctx); err != nil {
			unableToLoadDialog := nodewith.Name("Unable to load file").Role(role.Dialog)
			if loadFileErr := ui.Exists(unableToLoadDialog)(ctx); loadFileErr == nil {
				return errors.New("unable to load file")
			}
			testing.ContextLog(ctx, "Failed to wait for document saved within ", saveToDriveTimeout)
		} else {
			testing.ContextLog(ctx, "Saved to drive in ", time.Since(startTime))
		}
		return nil
	}
}

func waitForFieldTextToBe(tconn *chrome.TestConn, finder *nodewith.Finder, expectedText string) action.Action {
	ui := uiauto.New(tconn)
	return ui.WithInterval(400*time.Millisecond).RetrySilently(5,
		func(ctx context.Context) error {
			nodeInfo, err := ui.Info(ctx, finder)
			if err != nil {
				return err
			}
			if nodeInfo.Value != expectedText {
				return errors.Errorf("failed to validate input value: got: %s; want: %s", nodeInfo.Value, expectedText)
			}
			return nil
		})
}

// showTheMenus shows the hidden menu.
func showTheMenus(ui *uiauto.Context) action.Action {
	showTheMenusButton := nodewith.NameContaining("Show the menus").Role(role.Button)
	hideTheMenusButton := nodewith.NameContaining("Hide the menus").Role(role.Button)
	return uiauto.Combine("show the menus",
		ui.WaitUntilAnyExists(showTheMenusButton, hideTheMenusButton),
		uiauto.IfSuccessThen(ui.Exists(showTheMenusButton),
			ui.DoDefaultUntil(showTheMenusButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(hideTheMenusButton))),
	)
}

// DeleteDocsOrSlidesContent used for clean the test environment
func DeleteDocsOrSlidesContent(ctx context.Context, tconn *chrome.TestConn, appType string) error {
	var webArea = nodewith.NameContaining(appType).Role(role.RootWebArea)
	ui := uiauto.New(tconn)
	application := nodewith.Role(role.Application).Ancestor(webArea)
	menu := nodewith.Role(role.Menu).Ancestor(application)
	editButton := nodewith.Name("Edit").Role(role.MenuItem).Ancestor(application)
	selectAllMenuItem := nodewith.NameContaining("Select all").Role(role.MenuItem)
	delete := nodewith.NameContaining("Delete").Role(role.MenuItem)

	if err := uiauto.NamedCombine("delete all content",
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(selectAllMenuItem, ui.WithTimeout(shortUITimeout).WaitUntilGone(selectAllMenuItem)),
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(delete, ui.WithTimeout(shortUITimeout).WaitUntilGone(delete)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to delete all the content")
	}
	return nil
}

// clickGotItIfExists clicks "Got it" button if it exists.
func clickGotItIfExists(ui *uiauto.Context) action.Action {
	gotIt := nodewith.Name("Got it").First()
	return ui.RetryUntil(
		uiauto.IfSuccessThen(ui.Exists(gotIt), ui.DoDefault(gotIt)),
		ui.WithTimeout(3*time.Second).WaitUntilGone(gotIt),
	)
}

// reloadIfLoadingIssueDialogAppears reloads the page if the "Loading issue" dialog appears.
func reloadIfLoadingIssueDialogAppears(ui *uiauto.Context, webArea *nodewith.Finder) action.Action {
	application := nodewith.Role(role.Application).Ancestor(webArea)
	menuBar := nodewith.Name("Menu bar").Role(role.Banner).Ancestor(application)
	loadingIssueDialog := nodewith.Name("Loading issue").Role(role.Dialog).Ancestor(application)
	reloadNowButton := nodewith.Name("Reload now").Role(role.Button).Ancestor(loadingIssueDialog)
	reloadPage := uiauto.NamedCombine("reload page",
		ui.DoDefault(reloadNowButton),
		ui.WaitUntilGone(reloadNowButton),
		ui.WithTimeout(time.Minute).WaitUntilExists(menuBar),
		ui.EnsureGoneFor(loadingIssueDialog, 5*time.Second),
	)
	return uiauto.Retry(3, uiauto.IfSuccessThen(ui.Exists(loadingIssueDialog), reloadPage))
}

// getElementScreenCenter returns the center point of a DOM element in screen coordinates.
func getElementScreenCenter(ctx context.Context, conn *chrome.Conn, elementExpr string) (coords.Point, error) {
	var point struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}

	evalJS := fmt.Sprintf(`
		(() => {
			const el = %s;
			if (!el) return null;
			const r = el.getBoundingClientRect();
			return {
				x: window.screenX + r.left + r.width/2,
				y: window.screenY + (window.outerHeight-window.innerHeight) + r.top + r.height/2,
			};
		})()
	`, elementExpr)

	if err := conn.Eval(ctx, evalJS, &point); err != nil {
		return coords.Point{}, errors.Wrapf(err, "failed to calculate screen center for %q", elementExpr)
	}

	if point.X == 0 && point.Y == 0 {
		return coords.Point{}, errors.Errorf("failed to find element: %q", elementExpr)
	}
	return coords.Point{
		X: int(point.X),
		Y: int(point.Y),
	}, nil
}

func clickFileMenuButton(clickFileMenu action.Action, ui *uiauto.Context) action.Action {
	menuContainer := nodewith.Role(role.Menu).HasClass("shell-primary-menu").First()
	waitForFileMenu := ui.WithTimeout(10 * time.Second).WaitUntilExists(menuContainer)
	return uiauto.NamedAction("click file menu button",
		// If the File menu doesn't appear, maybe it's because the click
		// only focused the page. Then we just need to click again.
		ui.WithTimeout(time.Minute).RetryUntil(clickFileMenu, waitForFileMenu),
	)
}

// ClickFileMenuButtonWithJS clicks the "File" menu button using JS-calculated
// screen coordinates for accurate press and release metrics, since finder
// bounds are inaccurate.
func ClickFileMenuButtonWithJS(conn *chrome.Conn, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context) action.Action {
	const elementExpr = `document.querySelector("#docs-file-menu")`
	clickFileMenu := func(ctx context.Context) error {
		pt, err := getElementScreenCenter(ctx, conn, elementExpr)
		if err != nil {
			return err
		}
		return uiauto.Combine("click file menu button with js",
			mouse.Move(tconn, pt, 500*time.Millisecond),
			pc.ClickAt(pt),
		)(ctx)
	}
	return clickFileMenuButton(clickFileMenu, ui)
}

// ClickFileMenuButtonWithFinder clicks the "File" menu button using finder bounds
// for press and release metrics.
func ClickFileMenuButtonWithFinder(ui *uiauto.Context, pc pointer.Context) action.Action {
	fileMenu := nodewith.Name("File").Role(role.MenuItem).HasClass("menu-button").First()
	clickFileMenu := uiauto.Combine("click file menu button with finder",
		ui.MouseMoveTo(fileMenu, 500*time.Millisecond),
		pc.Click(fileMenu))
	return clickFileMenuButton(clickFileMenu, ui)

}
