// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps contains shared code used for app tests.
package apps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// https://stackoverflow.com/questions/45033015/how-do-i-turn-off-notifications-globally-in-visual-studio-code.
const disableNotificationsCommand = `cat << EOF >> /usr/share/%s/resources/app/out/vs/workbench/workbench.desktop.main.css
.monaco-workbench > .notifications-toasts.visible {
  display: none;
}

.notifications-toasts {
  display: none;
}
EOF
`

// VSCodeConfig describes how to interact with the program
type VSCodeConfig struct {
	Name         string
	AppName      string
	ConfigDir    string
	WindowFinder *nodewith.Finder
}

// VSCode is the official distribution
var VSCode = VSCodeConfig{
	Name:         "code",
	AppName:      "Visual Studio Code",
	ConfigDir:    "Code",
	WindowFinder: nodewith.NameContaining("Visual Studio Code").Role(role.Window).First(),
}

// VSCodium is a community distribution - https://vscodium.com/
var VSCodium = VSCodeConfig{
	Name:         "codium",
	AppName:      "VSCodium",
	ConfigDir:    "VSCodium",
	WindowFinder: nodewith.NameContaining("VSCodium").Role(role.Window).First(),
}

var (
	// VSCodeNewFile is the name of the file used to open VSCode for the first time.
	VSCodeNewFile = "new.go"
	// VSCodeTestFile is the name of the file where test input is entered.
	VSCodeTestFile = "test.go"
)

// Slower devices could take up to two minutes to start VSCode.
const vsCodeStartupTimeout = 2 * time.Minute

// InitialiseVSCode configures VS Code for testing by disabling cursor
// blinking, notifications, updates, and removing the "Getting Started" page.
func InitialiseVSCode(ctx context.Context, cfg VSCodeConfig, guest vm.Guest, uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) error {
	// Cursor blinking and vscode updates break screenshots.
	guest.WriteFile(ctx, fmt.Sprintf(".config/%s/User/settings.json", cfg.ConfigDir), `{"editor.cursorBlinking": "solid", "editor.unicodeHighlight.nonBasicASCII": "false", "workbench.startupEditor": "none", "update.mode": "none", "workbench.editor.untitled.hint": "hidden"}`)

	version, err := guest.Command(ctx, cfg.Name, "--version").Output()
	if err != nil {
		return errors.Wrap(err, "failed to check VS Code version")
	}
	testing.ContextLogf(ctx, "VS Code version: %s", string(version))

	if err := disableVSCodeNotifications(ctx, cfg, guest); err != nil {
		return err
	}

	if cfg.Name == "code" {
		// Even with the workbench.startupEditor set to None,
		// it still opens the Get Started tab when it is opened for the first time.
		// Therefore, open it and close it firstly.
		if err := launchAndCloseVSCode(cfg, uda, ui, terminalApp, keyboard)(ctx); err != nil {
			return err
		}
	}

	return nil
}

// disableVSCodeNotifications will disable VS Code notifications. It should be
// called before VS Code is launched.
func disableVSCodeNotifications(ctx context.Context, cfg VSCodeConfig, guest vm.Guest) error {
	// Sudo is required because the file the command modifies is read-only.
	cmd := guest.Command(ctx, "sudo", "sh", "-c", fmt.Sprintf(disableNotificationsCommand, cfg.Name))
	if _, err := cmd.Output(); err != nil {
		return errors.Wrapf(err, "failed to run %v", strings.Join(cmd.Args, " "))
	}
	return nil
}

// launchAndCloseVSCode will Launch the VS Code app from the terminal and close
// it without editing. This is used to dismiss the "Welcome" page.
func launchAndCloseVSCode(cfg VSCodeConfig, uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter) uiauto.Action {
	return uiauto.Combine("open VSCode for the first time",
		// Launch Visual Studio Code.
		terminalApp.RunCommand(keyboard, fmt.Sprintf("%s --disable-extensions", cfg.Name)),
		// Waiting for the welcome page, it always shows when opening the app for the first time.
		uda.WithTimeout(vsCodeStartupTimeout).WaitUntilExists(uidetection.Word("Welcome").WithinA11yNode(cfg.WindowFinder).First()),
		// Left click the app window header to focus.
		// Do not click the center of the app window, which may unexpectedly.
		// set the theme, see http://b/264336806.
		ui.LeftClick(nodewith.HasClass("HeaderView").Ancestor(cfg.WindowFinder)),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(cfg.WindowFinder))
}

// LaunchVSCodeForFile will launch the VS Code app from the terminal with the
// given file name and focus on the window for further input.
func LaunchVSCodeForFile(cfg VSCodeConfig, uda *uidetection.Context, ui *uiauto.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter, testFile string) uiauto.Action {
	vscodeUnsavedWindow := nodewith.NameStartingWith(fmt.Sprintf("● %s - %s", testFile, cfg.AppName)).Role(role.Window).First()
	return uiauto.Combine("Launch VS Code from the terminal",
		// Launch Visual Studio Code.
		terminalApp.RunCommand(keyboard, fmt.Sprintf("%s --disable-extensions %s", cfg.Name, testFile)),
		// Sometimes the first character got lost if input immediately.
		// Wait until the menu exists, indicating the window is launched.
		uda.WithTimeout(vsCodeStartupTimeout).WaitUntilExists(uidetection.Word("File").WithinA11yNode(cfg.WindowFinder).First()),
		// Also wait for the editor tab to load before further input.
		uda.WaitUntilExists(uidetection.Word(testFile).WithinA11yNode(cfg.WindowFinder).First()),
		// Left click the app window to wait for input.
		ui.LeftClick(nodewith.HasClass("HeaderView").Ancestor(vscodeUnsavedWindow)))
}

// SaveFileAndCloseVSCode will save the test file and close VS code app.
func SaveFileAndCloseVSCode(cfg VSCodeConfig, ui *uiauto.Context, keyboard *input.KeyboardEventWriter, testfile string) uiauto.Action {
	vscodeUnsavedWindow := nodewith.NameStartingWith(fmt.Sprintf("● %s - %s", testfile, cfg.AppName)).Role(role.Window).First()
	vscodeSavedWindow := nodewith.NameStartingWith(fmt.Sprintf("%s - %s", testfile, cfg.AppName)).Role(role.Window).First()
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

// TestCreateFileWithVSCode tests editing a file with VS code.
func TestCreateFileWithVSCode(ctx context.Context, cfg VSCodeConfig, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, guest vm.Guest, d screenshot.Differ) error {
	const testString = "//This is a test string."

	ui := uiauto.New(tconn)
	uda := uidetection.NewDefault(tconn)
	appWindowSaved := nodewith.NameStartingWith(fmt.Sprintf("%s - %s", VSCodeTestFile, cfg.AppName)).Role(role.Window).First()

	if err := InitialiseVSCode(ctx, cfg, guest, uda, ui, terminalApp, keyboard); err != nil {
		return err
	}

	saveAsWindow := nodewith.Name("Save As").HasClass("Widget").Role(role.Window)
	// UI interaction to save file.
	// File -> Save As -> Type file name -> Save.
	// This corresponds to step 5 at https://testtracker.googleplex.com/testplans/testcase/detail/4163083?id=18920&revision=232.
	saveFile := uiauto.Combine("save file from save as... dialogue",
		uda.LeftClick(uidetection.Word("File").WithinA11yNode(cfg.WindowFinder)),
		// "Save Workspace As...", "Save", and "Save As..." match the criteria, choose the third one.
		uda.LeftClick(uidetection.Word("Save").Nth(2)),
		uda.WaitUntilExists(uidetection.Word("Desktop").WithinA11yNode(saveAsWindow)),
		keyboard.AccelAction("ctrl+A"),
		keyboard.TypeAction(VSCodeTestFile),
		uda.LeftClick(uidetection.Word("Save").WithinA11yNode(saveAsWindow)),
	)

	// Open the VSCode again, this time, it won't open the Get Started tab.
	if err := uiauto.Combine("create file with VSCode",
		LaunchVSCodeForFile(cfg, uda, ui, terminalApp, keyboard, VSCodeNewFile),
		keyboard.TypeAction(testString),
		saveFile,
		ui.WaitUntilExists(appWindowSaved),
		screenshot.DiffWindow(ctx, d, "vscode", screenshot.Retries(5), screenshot.RetryInterval(5*time.Second)),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(appWindowSaved))(ctx); err != nil {
		return err
	}

	// Check the content of the test file.
	if err := guest.CheckFileContent(ctx, VSCodeTestFile, testString); err != nil {
		return errors.Wrap(err, "failed to verify the content of the file")
	}

	return nil
}

// OpenVscodeFromFileManager tests opening VS code from the context menu of a
// file in the files app.
func OpenVscodeFromFileManager(ctx context.Context, cfg VSCodeConfig, filesApp *filesapp.FilesApp, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, guest vm.Guest, dirName string) error {
	const tmpFilename = "testfile.txt"

	// Create a file in guest.
	if err := guest.Command(ctx, "touch", tmpFilename).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create a file in the guest")
	}

	// Right click and launch vscode through context menu.
	ui := uiauto.New(tconn)
	uda := uidetection.NewDefault(tconn)

	// Open tmp file with vscode.
	if err := uiauto.Combine("open tmp file with vscode via files app",
		filesApp.OpenDir(dirName, filesapp.FilesTitlePrefix+dirName),
		filesApp.ClickContextMenuItemRegex(tmpFilename, filesapp.OpenWith, cfg.AppName),
		// Wait until the window is stable.
		uda.WaitUntilExists(uidetection.Word("File").WithinA11yNode(cfg.WindowFinder).First()),
		// Left click the app window to focus.
		ui.LeftClick(cfg.WindowFinder),
		// Press ctrl+Q to exit window.
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(cfg.WindowFinder),
	)(ctx); err != nil {
		return err
	}

	return nil
}
