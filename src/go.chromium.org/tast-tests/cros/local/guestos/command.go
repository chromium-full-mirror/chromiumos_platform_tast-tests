// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
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

	ui := terminal.UI()
	vimEmptyLine := terminalapp.AsRow(nodewith.NameRegex(regexp.MustCompile(`^~\s*$`)).First())
	vimInsertMode := terminalapp.AsRow(nodewith.NameContaining("-- INSERT --").First())
	vimTypedLine := terminalapp.AsRow(nodewith.NameStartingWith(testString).First())
	vimSaveCmd := terminalapp.AsRow(nodewith.NameStartingWith(":x").First())

	if err := uiauto.Retry(3, func(ctx context.Context) error {
		// Clean up any lingering vim process or partial file from a previous attempt.
		_ = guest.Command(ctx, "pkill", "-9", "vim").Run()
		_ = guest.Command(ctx, "rm", "-f", testFile, "."+testFile+".swp").Run()
		if err := terminal.WaitForPromptWithTimeout(15 * time.Second)(ctx); err != nil {
			return errors.Wrap(err, "terminal prompt not ready before launching vim")
		}

		if err := uiauto.Combine("create a file with vim",
			// Open file through running command vim -n filename in Terminal.
			terminal.RunCommand(keyboard, fmt.Sprintf("vim -n %s", testFile)),
			// Wait for vim buffer screen to render before sending keys.
			ui.WithTimeout(15*time.Second).WaitUntilExists(vimEmptyLine),
			// Type i to enter edit mode and wait for -- INSERT -- indicator.
			keyboard.TypeAction("i"),
			ui.WithTimeout(10*time.Second).WaitUntilExists(vimInsertMode),
			// Type test string into the new file and wait for the full string to render
			// before pressing Esc, preventing late key events from arriving in Normal mode.
			keyboard.TypeAction(testString),
			ui.WithTimeout(15*time.Second).WaitUntilExists(vimTypedLine),
			// Press ESC to exit edit mode and wait until -- INSERT -- disappears.
			keyboard.AccelAction("Esc"),
			ui.WithTimeout(10*time.Second).WaitUntilGone(vimInsertMode),
			// Send an extra ESC to clear any stray pending normal-mode operator, then type :x.
			keyboard.AccelAction("Esc"),
			keyboard.TypeAction(":x"),
			ui.WithTimeout(10*time.Second).WaitUntilExists(vimSaveCmd),
			// Press Enter.
			keyboard.AccelAction("Enter"),
			// Wait for vim to exit.
			terminal.WaitForPromptWithTimeout(30*time.Second))(ctx); err != nil {
			return errors.Wrap(err, "failed to create file with vim in Terminal")
		}

		// Check the content of the test file.
		if err := guest.CheckFileContent(ctx, testFile, testString+"\n"); err != nil {
			return errors.Wrap(err, "the content of the file is wrong")
		}
		return nil
	})(ctx); err != nil {
		return err
	}

	return nil
}
