// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps contains shared code used for app tests.
package apps

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// WindowOperation is a test for window operations on an app.
type WindowOperation string

const (
	// WindowOperationMaximize tests maximize and restore.
	WindowOperationMaximize WindowOperation = "maximize"
	// WindowOperationMinimize tests minimize and restore.
	WindowOperationMinimize WindowOperation = "minimize"
	// WindowOperationSwitchTablet tests switching to tablet mode and back.
	WindowOperationSwitchTablet WindowOperation = "switch_tablet"
)

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

func diffWindowInTabletMode(ctx context.Context, tconn *chrome.TestConn, d screenshot.Differ, name string, options ...screenshot.Option) uiauto.Action {
	return func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
		defer cancel()

		revert, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
		if err != nil {
			return err
		}

		defer revert(cleanupCtx)

		return screenshot.DiffWindow(ctx, d, name, options...)(ctx)
	}
}

// TestWindowOperation tests the specified window operations for an application.
// TODO(b/298575662): HACK: Bruschetta windows are missing from the autotest
// APIs. Set skipReset to true to work around this if running on Bruschetta.
func TestWindowOperation(ctx context.Context, op WindowOperation, appName, appWindowName, appDetect string, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, d screenshot.Differ, skipReset bool) error {
	uda := uidetection.NewDefault(tconn).WithPollOpts(testing.PollOptions{Interval: time.Second, Timeout: 2 * time.Minute})
	ui := uiauto.New(tconn)
	appWindow := nodewith.NameRegex(regexp.MustCompile(appWindowName)).Role(role.Window).First()
	shelf := nodewith.Name("Shelf").Role(role.Toolbar).HasClass("ShelfView")

	resetNormalWindowState := func(ctx context.Context) error {
		if skipReset {
			return nil
		}
		window, err := ash.GetActiveWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "unable to find focused window")
		}
		return ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateNormal)
	}

	windowControlActions := map[WindowOperation]uiauto.Action{
		WindowOperationMaximize: uiauto.Combine("launch app and perform maximize/restore/close actions",
			launcher.SearchAndLaunchWithQuery(tconn, keyboard, appName, appName),
			// Wait until the window is stable.
			uda.WaitUntilExists(uidetection.Word(appDetect).First().WithinA11yNode(appWindow)),
			// Some apps (e.g, Firefox) may launch in maximized window by default on certain devices.
			resetNormalWindowState,
			Maximize(tconn, appWindow),
			screenshot.DiffWindow(ctx, d, "maximized", screenshot.Retries(5), screenshot.SkipSetWindowState(true)),
			RestoreFromMaximize(tconn, appWindow),
			screenshot.DiffWindow(ctx, d, "restored", screenshot.Retries(5), screenshot.SkipSetWindowState(true)),
			Close(tconn, appWindow),
			// Check the app disappears from the shelf.
			ui.WithTimeout(5*time.Second).WaitUntilGone(nodewith.NameContaining(appName).Role(role.Button).Visible().Ancestor(shelf)),
		),
		WindowOperationMinimize: uiauto.Combine("launch app and perform minimize/restore/close actions",
			launcher.SearchAndLaunchWithQuery(tconn, keyboard, appName, appName),
			// Wait until the window is stable.
			uda.WaitUntilExists(uidetection.Word(appDetect).First().WithinA11yNode(appWindow)),
			Minimize(tconn, appWindow),
			// Check that the app is still on the shelf.
			ui.Exists(nodewith.NameContaining(appName).Role(role.Button).Visible().Ancestor(shelf)),
			ShowFromShelf(tconn, appWindow, appName),
			Close(tconn, appWindow),
			// Check the app disappears from the shelf.
			ui.WithTimeout(5*time.Second).WaitUntilGone(nodewith.NameContaining(appName).Role(role.Button).Visible().Ancestor(shelf)),
		),
		WindowOperationSwitchTablet: uiauto.Combine("launch app and switch to tablet mode and back",
			launcher.SearchAndLaunchWithQuery(tconn, keyboard, appName, appName),
			// Wait until the window is stable.
			uda.WaitUntilExists(uidetection.Word(appDetect).First().WithinA11yNode(appWindow)),
			// Some apps (e.g, Firefox) may launch in maximized window by default on certain devices.
			resetNormalWindowState,
			diffWindowInTabletMode(ctx, tconn, d, "tablet", screenshot.Retries(5), screenshot.SkipSetWindowState(true)),
			screenshot.DiffWindow(ctx, d, "clamshell", screenshot.Retries(5), screenshot.SkipSetWindowState(true)),
			Close(tconn, appWindow),
			// Check the app disappears from the shelf.
			ui.WithTimeout(5*time.Second).WaitUntilGone(nodewith.NameContaining(appName).Role(role.Button).Visible().Ancestor(shelf)),
		),
	}

	action, ok := windowControlActions[op]
	if !ok {
		return errors.Errorf("no such window operation: %s", op)
	}

	testing.ContextLogf(ctx, "Running %s on app %q", op, appName)
	if err := action(ctx); err != nil {
		return errors.Wrapf(err, "failed to %s the app %s", op, appName)
	}

	return nil
}
