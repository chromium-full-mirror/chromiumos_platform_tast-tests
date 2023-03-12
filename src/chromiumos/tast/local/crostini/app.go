// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
	"go.chromium.org/tast/core/shutil"
)

func exitIfShown(ctx context.Context, tconn *chrome.TestConn, appID string) error {
	if visible, err := ash.AppShown(ctx, tconn, appID); err != nil {
		return err
	} else if !visible {
		return nil
	}
	return apps.Close(ctx, tconn, appID)
}

func findNewShelfItem(before, after []*ash.ShelfItem) (string, error) {
	if len(before) == len(after) {
		return "", errors.New("no new shelf item")
	}
	if len(before)+1 != len(after) {
		return "", errors.Errorf("item number mismatch, got %d wanted %d", len(after), len(before)+1)
	}
	beforeMap := map[string]bool{}
	for _, beforeItem := range before {
		beforeMap[beforeItem.AppID] = true
	}
	for _, afterItem := range after {
		if !beforeMap[afterItem.AppID] {
			return afterItem.AppID, nil
		}
	}
	return "", errors.New("could not find the new shelf item")
}

// LaunchGUIApp runs the given command, which is meant to be a crostini
// application with a GUI, and returns:
//   - A string, containing the ID of the app that was ran (i.e., a handle which
//     can be used to inspect/close the app).
//   - A callback which can be executed to close the application. Users of this
//     function should immediately defer the callback if one is returned.
//   - An error, which indicates something went wrong, or nil otherwise.
func LaunchGUIApp(ctx context.Context, tconn *chrome.TestConn, cmd *testexec.Cmd) (string, func(), error) {
	beforeItems, err := ash.ShelfItems(ctx, tconn)
	if err != nil {
		return "", func() {}, err
	}
	if err := cmd.Start(); err != nil {
		return "", func() {}, errors.Wrapf(err, "failed to start %q", shutil.EscapeSlice(cmd.Args))
	}
	var newID string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if currentItems, err := ash.ShelfItems(ctx, tconn); err != nil {
			return err
		} else if newID, err = findNewShelfItem(beforeItems, currentItems); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		cmd.Kill()
		cmd.Wait(testexec.DumpLogOnError)
		return "", func() {}, err
	}
	return newID, func() {
		exitIfShown(ctx, tconn, newID)
		cmd.Wait(testexec.DumpLogOnError)
	}, nil
}

// TakeAppScreenshot takes a screenshot and saves it to the test context output
// directory. On error will print a message and continue.
func TakeAppScreenshot(appName string) uiauto.Action {
	return func(ctx context.Context) error {
		dir, ok := testing.ContextOutDir(ctx)
		if !ok || dir == "" {
			testing.ContextLog(ctx, "Failed to get name of directory for screenshot")
		} else {
			path := filepath.Join(dir, "crostini_app_"+appName+".png")
			if err := screenshot.Capture(ctx, path); err != nil {
				testing.ContextLog(ctx, "Failed to take screenshot: ", err)
			}
		}
		return nil
	}
}

// Maximize returns an action that clicks the maximize button in the window
// title and waits until the restore button exists.
func Maximize(tconn *chrome.TestConn, windowFinder *nodewith.Finder) uiauto.Action {
	maximizeButton := nodewith.Name("Maximize").Role(role.Button).Ancestor(windowFinder)
	restoreButton := nodewith.Name("Restore").Role(role.Button).Ancestor(windowFinder)
	ui := uiauto.New(tconn)
	return ui.LeftClickUntil(
		maximizeButton,
		ui.WithTimeout(2*time.Second).WaitUntilExists(restoreButton),
	)
}

// RestoreFromMaximize returns an action that clicks the restore button in the
// window title and waits until the maximize button exists.
func RestoreFromMaximize(tconn *chrome.TestConn, windowFinder *nodewith.Finder) uiauto.Action {
	maximizeButton := nodewith.Name("Maximize").Role(role.Button).Ancestor(windowFinder)
	restoreButton := nodewith.Name("Restore").Role(role.Button).Ancestor(windowFinder)
	ui := uiauto.New(tconn)
	return ui.LeftClickUntil(
		restoreButton,
		ui.WithTimeout(2*time.Second).WaitUntilExists(maximizeButton),
	)
}

// Minimize returns an action that clicks the minimize button in the window
// title and waits until the app window becomes invisible.
func Minimize(tconn *chrome.TestConn, windowFinder *nodewith.Finder) uiauto.Action {
	minimizeButton := nodewith.Name("Minimize").Role(role.Button).Ancestor(windowFinder)
	ui := uiauto.New(tconn)
	return ui.LeftClickUntil(
		minimizeButton,
		ui.WithTimeout(2*time.Second).WaitUntilGone(windowFinder.Visible()),
	)
}

// ShowFromShelf returns an action that clicks the app icon in the Shelf to
// restore an app from minimized state.
func ShowFromShelf(tconn *chrome.TestConn, windowFinder *nodewith.Finder, appShelfName string) uiauto.Action {
	shelf := nodewith.Name("Shelf").Role(role.Toolbar).HasClass("ShelfView")
	appShelfButton := nodewith.NameContaining(appShelfName).Role(role.Button).Visible().Ancestor(shelf)
	ui := uiauto.New(tconn)
	return ui.LeftClickUntil(
		appShelfButton,
		ui.WithTimeout(2*time.Second).WaitUntilExists(windowFinder.Visible()),
	)
}

// Close returns an action that clicks the close button in the window title and
// waits until the app window is gone.
func Close(tconn *chrome.TestConn, windowFinder *nodewith.Finder) uiauto.Action {
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(windowFinder)
	ui := uiauto.New(tconn)
	return ui.LeftClickUntil(
		closeButton,
		ui.WithTimeout(2*time.Second).WaitUntilGone(windowFinder),
	)
}
