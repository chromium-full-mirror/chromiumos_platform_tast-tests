// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// SetupEmacs configures emacs for testing.
func SetupEmacs(ctx context.Context, guest vm.Guest) error {
	// Anything animated like blinking cursors will break screendiffs.
	// Set the window title to "GNU Emacs" so it can be identified
	// regardless of emacs version.
	// Avoid opening the splash screen since it means the screenshot will
	// contain data like CPU architecture, which is terrible for screen
	// diffing.
	return guest.WriteFile(ctx, "~/.emacs", `(blink-cursor-mode 0)
(setq-default frame-title-format '("GNU Emacs"))
(setq inhibit-startup-message t)`)
}

// CreateFileWithEmacs opens the terminal, then launches GUI emacs and sends
// keystrokes to it in order to type something into a file, then verifies what
// was written.
func CreateFileWithEmacs(ctx context.Context, keyboard *input.KeyboardEventWriter, terminal *terminalapp.TerminalApp, tconn *chrome.TestConn, guest vm.Guest, d screenshot.Differ) error {
	const (
		testFile   = "test.txt"
		testString = "This is a test string"
	)

	if err := SetupEmacs(ctx, guest); err != nil {
		return errors.Wrap(err, "failed to setup emacs for testing")
	}

	// Open emacs in Terminal.
	if err := terminal.RunCommand(keyboard, fmt.Sprintf("emacs %s", testFile))(ctx); err != nil {
		return errors.Wrap(err, "failed to run command 'emacs' in Terminal window")
	}

	ui := uiauto.New(tconn)
	uda := uidetection.NewDefault(tconn)
	window := nodewith.NameContaining("GNU Emacs").Role(role.Window).First()

	if err := uiauto.Combine("Click, input, save and exit Emacs",
		// Sometimes the first character got lost if input immediately.
		// Wait until the menu exists, indicating the window is launched.
		uda.WaitUntilExists(uidetection.Word("File").WithinA11yNode(window).First()),
		// Type string.
		keyboard.TypeAction(testString),
		// Press ctrl+x and ctrl+s to save.
		keyboard.AccelAction("ctrl+X"),
		keyboard.AccelAction("ctrl+S"),
		// After saving, wait for the "save" button to grey out.
		screenshot.DiffWindow(ctx, d, "emacs"),
		// Press ctrl+x and ctrl+c to and quit.
		keyboard.AccelAction("ctrl+X"),
		keyboard.AccelAction("ctrl+C"),
		// Check window closed.
		ui.WaitUntilGone(window))(ctx); err != nil {
		return err
	}

	// Check the content of the test file.
	if err := guest.CheckFileContent(ctx, testFile, testString+"\n"); err != nil {
		return errors.Wrap(err, "failed to verify the content of the file")
	}
	return nil
}
