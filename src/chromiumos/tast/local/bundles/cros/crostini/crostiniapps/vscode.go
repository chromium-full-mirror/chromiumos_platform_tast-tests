// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crostiniapps contains shared code used for app tests.
package crostiniapps

import (
	"context"
	"fmt"
	"strings"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/terminalapp"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/local/vm"
	"chromiumos/tast/testing"
)

// https://stackoverflow.com/questions/45033015/how-do-i-turn-off-notifications-globally-in-visual-studio-code.
const disableNotificationsCommand = `cat << EOF >> /usr/share/code/resources/app/out/vs/workbench/workbench.desktop.main.css
.monaco-workbench > .notifications-toasts.visible {
  display: none;
}

.notifications-toasts {
  display: none;
}
EOF
`

var (
	// VscodeWindow is a finder for the VS Code app window.
	VscodeWindow = nodewith.NameContaining("Visual Studio Code").Role(role.Window).First()
	// VscodeNewFile is the name of the file used to open VSCode for the first time.
	VscodeNewFile = "new.go"
	// VscodeTestFile is the name of the file where test input is entered.
	VscodeTestFile = "test.go"
)

// InitialiseVscode configures VS Code for testing by disabling cursor blinking, notifications, updates, and removing the "Getting Started" page.
func InitialiseVscode(ctx context.Context, cont *vm.Container, uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) error {
	// Cursor blinking and vscode updates break screenshots.
	cont.WriteFile(ctx, ".config/Code/User/settings.json", `{"editor.cursorBlinking": "solid", "editor.unicodeHighlight.nonBasicASCII": "false", "workbench.startupEditor": "None", "update.mode": "none", "workbench.editor.untitled.hint": "hidden"}`)

	version, err := cont.Command(ctx, "code", "--version").Output()
	if err != nil {
		return errors.Wrap(err, "failed to check VS Code version")
	}
	testing.ContextLogf(ctx, "VS Code version: %s", string(version))

	if err := disableVscodeNotifications(ctx, cont); err != nil {
		return err
	}

	// Even with the workbench.startupEditor set to None,
	// it still opens the Get Started tab when it is opened for the first time.
	// Therefore, open it and close it firstly.
	if err := launchAndCloseVscode(uda, ui, terminalApp, keyboard)(ctx); err != nil {
		return err
	}

	return nil
}

// disableVscodeNotifications will disable VS Code notifications. It should be called before VS Code is launched.
func disableVscodeNotifications(ctx context.Context, cont *vm.Container) error {
	// Sudo is required because the file the command modifies is read-only.
	cmd := cont.Command(ctx, "sudo", "sh", "-c", disableNotificationsCommand)
	if _, err := cmd.Output(); err != nil {
		return errors.Wrapf(err, "failed to run %v", strings.Join(cmd.Args, " "))
	}
	return nil
}

// launchAndCloseVscode will Launch the VS Code app from the terminal with a "new.go" file and close it without editing. This is used to removed the "Getting Started" page.
func launchAndCloseVscode(uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) uiauto.Action {
	return uiauto.Combine("open VSCode for the first time",
		// Launch Visual Studio Code.
		terminalApp.RunCommand(keyboard, fmt.Sprintf("code --disable-extensions %s", VscodeTestFile)),
		// Sometimes the first character got lost if input immediately.
		// Wait until the menu exists, indicating the window is launched.
		uda.WaitUntilExists(uidetection.Word("File").WithinA11yNode(VscodeWindow).First()),
		// Left click the app window header to focus.
		// Do not click the center of the app window, which may unexpectedly
		// set the theme, see http://b/264336806.
		ui.LeftClick(nodewith.HasClass("HeaderView").Ancestor(VscodeWindow)),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(VscodeWindow))
}

// LaunchVscodeForFile will launch the VS Code app from the terminal with the given file name and focus on the window for further input.
func LaunchVscodeForFile(uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter, testFile string) uiauto.Action {
	vscodeUnsavedWindow := nodewith.NameStartingWith(fmt.Sprintf("● %s - Visual Studio Code", testFile)).Role(role.Window).First()
	return uiauto.Combine("Launch VS Code from the terminal",
		// Launch Visual Studio Code.
		terminalApp.RunCommand(keyboard, fmt.Sprintf("code --disable-extensions %s", testFile)),
		// Sometimes the first character got lost if input immediately.
		// Wait until the menu exists, indicating the window is launched.
		uda.WaitUntilExists(uidetection.Word("File").WithinA11yNode(VscodeWindow).First()),
		// Left click the app window to wait for input
		ui.LeftClick(nodewith.HasClass("HeaderView").Ancestor(vscodeUnsavedWindow)))
}

// SaveFileAndCloseVscode will save the test file and close VS code app.
func SaveFileAndCloseVscode(ui *uiauto.Context, keyboard *input.KeyboardEventWriter, testfile string) uiauto.Action {
	vscodeSavedWindow := nodewith.NameStartingWith(fmt.Sprintf("● %s - Visual Studio Code", testfile)).Role(role.Window).First()
	return uiauto.Combine("Saving file named %s and closing VS Code",
		// Save the file
		keyboard.AccelAction("Ctrl+S"),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(vscodeSavedWindow),
	)
}
