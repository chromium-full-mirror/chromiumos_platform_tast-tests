// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// CommandVim creates a file using vim and checks the file contents.
func CommandVim(ctx context.Context, terminal *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter, guest vm.Guest) error {
	// Check that vim is preinstalled.
	if err := guest.Command(ctx, "vim", "--version").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to execute vim")
	}

	const (
		testFile   = "command_vim.txt"
		testString = "This is a test string."
	)

	if err := uiauto.Combine("create a file with vim",
		// Open file through running command vim filename in Terminal.
		terminal.RunCommand(keyboard, fmt.Sprintf("vim %s", testFile)),
		// Type i to enter edit mode.
		keyboard.TypeAction("i"),
		// Type test string into the new file.
		keyboard.TypeAction(testString),
		// Press ESC to exit edit mode.
		keyboard.TypeAction(string('\x1b')),
		// Type :x to save.
		keyboard.TypeAction(":x"),
		// Press Enter.
		keyboard.AccelAction("Enter"),
		// Wait for vim to exit
		terminal.WaitForPrompt())(ctx); err != nil {
		return errors.Wrap(err, "failed to create file with vim in Terminal")
	}

	// Check the content of the test file.
	if err := guest.CheckFileContent(ctx, testFile, testString+"\n"); err != nil {
		return errors.Wrap(err, "the content of the file is wrong")
	}

	return nil
}
