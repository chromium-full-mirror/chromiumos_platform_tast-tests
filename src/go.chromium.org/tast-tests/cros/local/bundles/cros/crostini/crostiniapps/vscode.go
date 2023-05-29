// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crostiniapps contains shared code used for app tests.
package crostiniapps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
	// VSCodeWindow is a finder for the VS Code app window.
	VSCodeWindow = nodewith.NameContaining("Visual Studio Code").Role(role.Window).First()
	// VSCodeNewFile is the name of the file used to open VSCode for the first time.
	VSCodeNewFile = "new.go"
	// VSCodeTestFile is the name of the file where test input is entered.
	VSCodeTestFile = "test.go"
)

// Slower devices could take up to two minutes to start Firefox.
const vsCodeStartupTimeout = 2 * time.Minute

// InitialiseVSCode configures VS Code for testing by disabling cursor blinking, notifications, updates, and removing the "Getting Started" page.
func InitialiseVSCode(ctx context.Context, cont *vm.Container, uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) error {
	// Cursor blinking and vscode updates break screenshots.
	cont.WriteFile(ctx, ".config/Code/User/settings.json", `{"editor.cursorBlinking": "solid", "editor.unicodeHighlight.nonBasicASCII": "false", "workbench.startupEditor": "none", "update.mode": "none", "workbench.editor.untitled.hint": "hidden"}`)

	version, err := cont.Command(ctx, "code", "--version").Output()
	if err != nil {
		return errors.Wrap(err, "failed to check VS Code version")
	}
	testing.ContextLogf(ctx, "VS Code version: %s", string(version))

	if err := disableVSCodeNotifications(ctx, cont); err != nil {
		return err
	}

	// Even with the workbench.startupEditor set to None,
	// it still opens the Get Started tab when it is opened for the first time.
	// Therefore, open it and close it firstly.
	if err := launchAndCloseVSCode(uda, ui, terminalApp, keyboard)(ctx); err != nil {
		return err
	}

	return nil
}

// disableVSCodeNotifications will disable VS Code notifications. It should be called before VS Code is launched.
func disableVSCodeNotifications(ctx context.Context, cont *vm.Container) error {
	// Sudo is required because the file the command modifies is read-only.
	cmd := cont.Command(ctx, "sudo", "sh", "-c", disableNotificationsCommand)
	if _, err := cmd.Output(); err != nil {
		return errors.Wrapf(err, "failed to run %v", strings.Join(cmd.Args, " "))
	}
	return nil
}

// launchAndCloseVSCode will Launch the VS Code app from the terminal with a "new.go" file and close it without editing. This is used to removed the "Getting Started" page.
func launchAndCloseVSCode(uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) uiauto.Action {
	return uiauto.Combine("open VSCode for the first time",
		// Launch Visual Studio Code.
		terminalApp.RunCommand(keyboard, fmt.Sprintf("code --disable-extensions %s", VSCodeNewFile)),
		// Waiting for the welcome page, it always shows when opening the app for the first time.
		uda.WithTimeout(vsCodeStartupTimeout).WaitUntilExists(uidetection.Word("Welcome").WithinA11yNode(VSCodeWindow).First()),
		// Left click the app window header to focus.
		// Do not click the center of the app window, which may unexpectedly.
		// set the theme, see http://b/264336806.
		ui.LeftClick(nodewith.HasClass("HeaderView").Ancestor(VSCodeWindow)),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(VSCodeWindow))
}

// LaunchVSCodeForFile will launch the VS Code app from the terminal with the given file name and focus on the window for further input.
func LaunchVSCodeForFile(uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter, testFile string) uiauto.Action {
	vscodeUnsavedWindow := nodewith.NameStartingWith(fmt.Sprintf("● %s - Visual Studio Code", testFile)).Role(role.Window).First()
	return uiauto.Combine("Launch VS Code from the terminal",
		// Launch Visual Studio Code.
		terminalApp.RunCommand(keyboard, fmt.Sprintf("code --disable-extensions %s", testFile)),
		// Sometimes the first character got lost if input immediately.
		// Wait until the menu exists, indicating the window is launched.
		uda.WithTimeout(vsCodeStartupTimeout).WaitUntilExists(uidetection.Word("File").WithinA11yNode(VSCodeWindow).First()),
		// Also wait for the editor tab to load before further input.
		uda.WaitUntilExists(uidetection.Word(testFile).WithinA11yNode(VSCodeWindow).First()),
		// Left click the app window to wait for input.
		ui.LeftClick(nodewith.HasClass("HeaderView").Ancestor(vscodeUnsavedWindow)))
}

// SaveFileAndCloseVSCode will save the test file and close VS code app.
func SaveFileAndCloseVSCode(ui *uiauto.Context, keyboard *input.KeyboardEventWriter, testfile string) uiauto.Action {
	vscodeUnsavedWindow := nodewith.NameStartingWith(fmt.Sprintf("● %s - Visual Studio Code", testfile)).Role(role.Window).First()
	vscodeSavedWindow := nodewith.NameStartingWith(fmt.Sprintf("%s - Visual Studio Code", testfile)).Role(role.Window).First()
	return uiauto.Combine("Saving file named %s and closing VS Code",
		// Save the file.
		keyboard.AccelAction("Ctrl+S"),
		// Wait for the unsaved window should be gone to confirm that the file has been saved.
		ui.WaitUntilGone(vscodeUnsavedWindow),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(vscodeSavedWindow),
	)
}
