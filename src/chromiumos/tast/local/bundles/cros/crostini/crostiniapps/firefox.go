// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crostiniapps contains shared code used for app tests.
package crostiniapps

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/terminalapp"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/local/vm"

	"go.chromium.org/tast/core/errors"
)

// Firefox test page content. Contains a single input box which is focused upon launch for user input.
const html = `<!DOCTYPE html>
<title>Crostini Firefox Input Test Page</title>
<input style="width: 100%" autofocus/>
`

// Slower devices could take up to a minute to start Firefox.
const firefoxStartupTimeout = time.Minute

const testPageName = "test_page.html"

var firefoxWindow = nodewith.NameRegex(regexp.MustCompile(`.*Mozilla Firefox`)).Role(role.Window).First()

// LaunchFirefoxWithTestPage creates a test webpage and launches it with the Firefox app.
func LaunchFirefoxWithTestPage(ctx context.Context, uda *uidetection.Context, ui *uiauto.Context, cont *vm.Container, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) error {
	// Create a file with test page.
	if err := cont.WriteFile(ctx, testPageName, html); err != nil {
		return err
	}

	if err := uiauto.Combine("create test page in terminal and open in firefox",
		terminalApp.RunCommand(keyboard, "firefox-esr "+testPageName),
		ui.WithTimeout(firefoxStartupTimeout).WaitUntilExists(firefoxWindow),
		// Wait until the page is loaded.
		uda.WaitUntilExists(uidetection.TextBlock([]string{"Crostini", "Firefox", "Input", "Test", "Page"}).WithinA11yNode(firefoxWindow).First()),
	)(ctx); err != nil {
		return err
	}
	return nil
}

// CloseFirefoxTestPage removes the testfile and closes the Firefox webpage using keyboard shortcuts, then waits for the window to close.
func CloseFirefoxTestPage(ctx context.Context, ui *uiauto.Context, cont *vm.Container, keyboard *input.KeyboardEventWriter) error {
	// Firefox may open up another default page like "Privacy Notice", so press ctrl+w twice before checking the page is gone.
	if err := uiauto.Combine("close webpage",
		keyboard.AccelAction("ctrl+W"),
		keyboard.AccelAction("ctrl+W"),
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
