// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crostiniapps contains shared code used for app tests.
package crostiniapps

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
)

// Firefox test page content. Contains a single input box which is focused upon launch for user input. The test page contains two input boxes to test tab behaviour.
const html = `<!DOCTYPE html>
<title>Crostini Firefox Input Test Page</title>
<textarea style="width: 100%" autofocus></textarea>
<br>
<input style="width: 100%" />
`

// Slower devices could take up to two minutes to start Firefox.
const firefoxStartupTimeout = 2 * time.Minute
const testPageName = "test_page.html"

var firefoxWindow = nodewith.NameRegex(regexp.MustCompile(`.*Mozilla Firefox`)).Role(role.Window).First()
var testPageTab = uidetection.TextBlock([]string{"Crostini", "Firefox", "Input", "Test", "Page"}).WithinA11yNode(firefoxWindow).First()
var maximizeButton = nodewith.Name("Maximize").Role(role.Button).Ancestor(firefoxWindow)

// LaunchFirefoxWithTestPage creates a test webpage and launches it with the Firefox app.
func LaunchFirefoxWithTestPage(ctx context.Context, tconn *chrome.TestConn, uda *uidetection.Context, ui *uiauto.Context, cont *vm.Container, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) error {
	// Create a file with test page.
	if err := cont.WriteFile(ctx, testPageName, html); err != nil {
		return err
	}

	maximizeWindow := func(ctx context.Context) error {
		// Maximise the firefox window so that elements can be found easily.
		activeWindow, err := ash.GetActiveWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get active window")
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, activeWindow.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize Chrome window")
		}
		return nil
	}

	if err := uiauto.Combine("create test page in terminal and open in firefox",
		terminalApp.RunCommand(keyboard, "firefox-esr "+testPageName),
		ui.WithTimeout(firefoxStartupTimeout).WaitUntilExists(firefoxWindow),
		maximizeWindow,
		// Wait until the page is loaded.
		uda.WithScreenshotStrategy(uidetection.ImmediateScreenshot).WaitUntilExists(testPageTab),
	)(ctx); err != nil {
		return err
	}

	return nil
}

// CloseFirefoxTestPage removes the testfile and closes the Firefox webpage using keyboard shortcuts, then waits for the window to close.
func CloseFirefoxTestPage(ctx context.Context, uda *uidetection.Context, ui *uiauto.Context, cont *vm.Container, keyboard *input.KeyboardEventWriter) error {
	// Firefox may open up another default page like "Privacy Notice", so keep pressing ctrl+w twice before checking the window is gone.
	if err := ui.WithInterval(time.Second).RetryUntil(
		keyboard.AccelAction("ctrl+w"),
		ui.WithTimeout(3*time.Second).WaitUntilGone(firefoxWindow),
	)(ctx); err != nil {
		return err
	}

	// Remove the test page file.
	if err := cont.RemoveAll(ctx, testPageName); err != nil {
		return errors.Errorf("failed to remove file %s: %s", testPageName, err)
	}
	return nil
}

// VerifyFirefoxLaunchAndClose verifies that firefox window exists and firefox on shelf.
// Firefox must have been launched.
// It also closes firefox.
func VerifyFirefoxLaunchAndClose(ctx context.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter) error {
	const startupTimeout = 2 * time.Minute // slower devices could take up to two minutes to start Firefox.
	ud := uidetection.NewDefault(tconn)
	ui := uiauto.New(tconn).WithTimeout(startupTimeout)
	firefoxWindow := nodewith.NameRegex(regexp.MustCompile(`.*Mozilla Firefox`)).Role(role.Window).First()

	// Check firefox window.
	if err := uiauto.Combine("verify Firefox",
		ui.WaitUntilExists(firefoxWindow),
		uiauto.IfFailThen(
			ud.WaitUntilExists(uidetection.TextBlock([]string{"Welcome", "to", "Firefox"}).WithinA11yNode(firefoxWindow).First()),
			ud.WaitUntilExists(uidetection.TextBlock([]string{"Get", "started"}).WithinA11yNode(firefoxWindow).First()),
		),
	)(ctx); err != nil {
		return err
	}

	// Check firefox is displayed on shelf.
	shelfItems, err := ash.ShelfItems(ctx, tconn)
	found := false
	if err != nil {
		return errors.Wrap(err, "failed to get shelf items")
	}
	for _, shelfItem := range shelfItems {
		if shelfItem.Title == "Firefox ESR" {
			found = true
			break
		}
	}
	if !found {
		return errors.New("failed to find firefox on shelf")
	}

	// Close firefox.
	if err := ui.WithInterval(time.Second).RetryUntil(
		keyboard.AccelAction("ctrl+w"),
		ui.WithTimeout(3*time.Second).WaitUntilGone(firefoxWindow),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close firefox")
	}

	return nil
}
