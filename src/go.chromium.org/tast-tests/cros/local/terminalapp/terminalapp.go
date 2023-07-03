// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package terminalapp supports actions on Terminal on ChromeOS.
package terminalapp

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	bruconstants "go.chromium.org/tast-tests/cros/local/bruschetta/constants"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const uiTimeout = 15 * time.Second

// LaunchTerminalTimeout is the timeout for launching Terminal. It normally
// takes a few seconds after running Crostini. However, on some devices
// launching Terminal when restarting Crostini is unexpectedly slow, which
// may take longer than one minute (e.g., see b/240366940).
// Note: This is NOT the expected time to launch Terminal, but a relaxation
// for extreme cases to avoid test failures due to the timeout.
// The performance issue will be tracked in future performance testing.
const LaunchTerminalTimeout = 2 * time.Minute

var (
	linuxLink           = nodewith.Name("penguin").Role(role.Link)
	bruschettaLink      = nodewith.Name(bruconstants.BruschettaVMName).Role(role.Link)
	linuxTab            = nodewith.NameContaining("@penguin: ").Role(role.Window).ClassName("BrowserFrame")
	bruschettaTab       = nodewith.NameContaining("chronos@localhost: ").Role(role.Window).ClassName("BrowserFrame")
	sshTab              = nodewith.NameContaining("chronos@localhost:").Role(role.Window).ClassName("BrowserFrame")
	rootWindow          = nodewith.NameStartingWith("Terminal").Role(role.Window).ClassName("BrowserFrame")
	homeTab             = nodewith.Name("Terminal").Role(role.Window).ClassName("BrowserFrame")
	terminalLeaveButton = nodewith.Name("Leave").Role(role.Button).HasClass("MdTextButton")
	terminalTextField   = nodewith.Name("Terminal input").Role(role.TextField)

	webArea = nodewith.NameRegex(regexp.MustCompile(`\@penguin\: `)).Role(role.RootWebArea)
	// Prompt is the input prefix.
	Prompt = AsRow(nodewith.NameRegex(regexp.MustCompile(`\$\s*$`)))

	// CmdPrompt is the input prefix of tmux tabs.
	CmdPrompt = Row("chronos@localhost ~ $")
	// TmuxModeMsg represents the controlling tab of Tmux.
	TmuxModeMsg = AsRow(nodewith.NameStartingWith("Tmux integration mode activated"))
)

// TerminalApp represents an instance of the Terminal App.
type TerminalApp struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
	Kb    *input.KeyboardEventWriter
	tab   *nodewith.Finder
}

func launch(ctx context.Context, tconn *chrome.TestConn, link, tab *nodewith.Finder) (*TerminalApp, error) {
	// Launch the Terminal App.
	if err := apps.Launch(ctx, tconn, apps.Terminal.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch the Terminal App through package apps")
	}
	ta, err := find(ctx, tconn, link, tab)
	if err != nil {
		if closeErr := apps.Close(ctx, tconn, apps.Terminal.ID); closeErr != nil {
			testing.ContextLog(ctx, "Error closing terminal app: ", closeErr)
		}
		return nil, errors.Wrap(err, "failed to find the Terminal App")
	}
	return ta, nil
}

func find(ctx context.Context, tconn *chrome.TestConn, link, tab *nodewith.Finder) (*TerminalApp, error) {
	ui := uiauto.New(tconn)

	// Find VM tab.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// If found Home tab with VM link, click the link to switch to
		// or create the VM tab.
		if err := ui.Exists(link)(ctx); err == nil {
			if err := ui.DoDefault(link)(ctx); err != nil {
				return errors.Wrap(err, "failed to click Terminal Home Linux")
			}
		}
		if err := ui.Exists(tab)(ctx); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: LaunchTerminalTimeout}); err != nil {
		return nil, errors.Wrap(err, "failed to find the Terminal App window")
	}

	// Check Terminal is on shelf.
	if err := ash.WaitForApp(ctx, tconn, apps.Terminal.ID, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to find Terminal icon on shelf")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}

	terminalApp := &TerminalApp{tconn: tconn, ui: ui, Kb: kb, tab: tab}
	if err := terminalApp.WaitForPrompt()(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for terminal prompt")
	}

	return terminalApp, nil
}

// Launch launches the Terminal App connected to default penguin container and returns it.
// An error is returned if the app fails to launch.
func Launch(ctx context.Context, tconn *chrome.TestConn) (*TerminalApp, error) {
	return launch(ctx, tconn, linuxLink, linuxTab)
}

// Find finds an open Terminal App and opens a Linux tab if not already open.
// An error is returned if terminal cannot be found.
func Find(ctx context.Context, tconn *chrome.TestConn) (*TerminalApp, error) {
	return find(ctx, tconn, linuxLink, linuxTab)
}

// LaunchBruschetta launches the Terminal App connected to the default bruschetta VM and returns it.
// An error is returned if the app fails to launch.
func LaunchBruschetta(ctx context.Context, tconn *chrome.TestConn) (*TerminalApp, error) {
	return launch(ctx, tconn, bruschettaLink, bruschettaTab)
}

// FindBruschetta finds an open Terminal App and opens a connection to the default bruschetta VM and returns it.
// An error is returned if terminal cannot be found.
func FindBruschetta(ctx context.Context, tconn *chrome.TestConn) (*TerminalApp, error) {
	return find(ctx, tconn, bruschettaLink, bruschettaTab)
}

// LaunchSSH launches Terminal App and connects to chronos@localhost.
// with the optional sshArgs. An error is returned if the app fails to launch.
// Sets IME to en-US in order to send @ symbol correctly as Shift-2.
func LaunchSSH(ctx context.Context, tconn *chrome.TestConn, sshArgs string) (*TerminalApp, error) {
	// Launch the Terminal App.
	if err := apps.Launch(ctx, tconn, apps.Terminal.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch the Terminal App through package apps")
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}

	ui := uiauto.New(tconn)
	var ta = &TerminalApp{tconn: tconn, ui: ui, Kb: kb, tab: sshTab}

	if err := uiauto.Combine("launch ssh",
		ta.SetUpSSHConnection(sshArgs),
		ta.OpenSSHConnection(),
	)(ctx); err != nil {
		return nil, err
	}

	return ta, nil
}

// dismissLeaveAppDialogIfShown dimisses the "Leave app?" dialog if shown.
func (ta *TerminalApp) dismissLeaveAppDialogIfShown() uiauto.Action {
	return uiauto.IfSuccessThen(
		ta.ui.WithTimeout(time.Second).WaitUntilExists(terminalLeaveButton),
		ta.ui.LeftClickUntil(
			terminalLeaveButton,
			ta.ui.WithTimeout(time.Second).WaitUntilGone(terminalLeaveButton),
		),
	)
}

// SetUpSSHConnection sets up a ssh connection to chronos@localhost.
func (ta *TerminalApp) SetUpSSHConnection(sshArgs string) uiauto.Action {
	cmd := "chronos@localhost -o StrictHostKeyChecking=no " + sshArgs
	relay := "--ssh-client-version=pnacl"
	addSSH := uiauto.Combine("input ssh information",
		ta.ui.LeftClick(nodewith.Name("Add SSH").Role(role.Button)),
		ta.ui.LeftClickUntilFocused(nodewith.Name("Command").Role(role.TextField)),
		ta.Kb.TypeAction(cmd),
		ta.ui.LeftClickUntilFocused(nodewith.Name("SSH relay server options").Role(role.TextField)),
		ta.Kb.TypeAction(relay),
		ta.ui.WithTimeout(2*time.Second).WaitUntilExists(nodewith.Role(role.StaticText).Name(cmd)),
		ta.ui.WithTimeout(2*time.Second).WaitUntilExists(nodewith.Role(role.StaticText).Name(relay)))

	save := ta.ui.LeftClick(nodewith.Name("Save").Role(role.Button))
	cancel := ta.ui.LeftClick(nodewith.Name("Cancel").Role(role.Button))

	return uiauto.Combine("set up ssh connection",
		ime.EnglishUS.InstallAndActivate(ta.tconn),
		ta.DeleteSSHConnection("chronos@localhost"),
		uiauto.Retry(3, uiauto.Combine("type ssh configs",
			uiauto.IfSucceedThenElse(addSSH, save, cancel),
			ta.ui.WithTimeout(2*time.Second).WaitUntilExists(nodewith.Name("chronos@localhost").Role(role.Link))),
		),
	)
}

// OpenSSHConnection opens the ssh connection set up by SetUpSSHConnection().
func (ta *TerminalApp) OpenSSHConnection() uiauto.Action {
	return uiauto.Combine("open the ssh connection",
		ta.ui.LeftClick(nodewith.Name("chronos@localhost").Role(role.Link)),
		ta.ui.LeftClick(nodewith.Name("(chronos@localhost) Password:").Role(role.TextField)),
		ta.Kb.TypeAction("test0000"),
		ta.Kb.AccelAction("Enter"),
		ta.ui.WaitUntilExists(nodewith.Name("chronos@localhost ~ $").Role(role.StaticText).First()),
	)
}

// DeleteSSHConnection deletes the specified connection link if it exists.
func (ta *TerminalApp) DeleteSSHConnection(name string) uiauto.Action {
	l := nodewith.Name(name).Role(role.Link)
	return action.IfSuccessThen(ta.ui.WithTimeout(3*time.Second).WaitUntilExists(l),
		uiauto.Combine("delete ssh link "+name,
			ta.ui.LeftClick(nodewith.Name("Options").Role(role.Button)),
			ta.ui.LeftClick(nodewith.Name("Remove").Role(role.StaticText)),
			ta.ui.LeftClick(nodewith.Name("OK").Role(role.Button).Ancestor(nodewith.Role(role.Dialog))),
			ta.ui.WaitUntilGone(l),
		))
}

// RunSSHCommand runs command in Terminal SSH tab.
func (ta *TerminalApp) RunSSHCommand(cmd string) uiauto.Action {
	return uiauto.Combine("run command "+cmd,
		ta.Kb.TypeAction(cmd),
		ta.Kb.AccelAction("Enter"),
	)
}

// ExitSSH exits the current connection and closes the app.
func (ta *TerminalApp) ExitSSH() uiauto.Action {
	return uiauto.Combine("exit ssh",
		ta.RunSSHCommand("exit"),
		ta.ui.WaitUntilExists(nodewith.NameRegex(regexp.MustCompile(`Connection to \S+ closed.`)).Role(role.StaticText).First()),
		ta.Kb.AccelAction("Esc"),
		ta.dismissLeaveAppDialogIfShown(),
		ta.Kb.AccelAction("Ctrl+Shift+W"),
		ta.dismissLeaveAppDialogIfShown(),
	)
}

// WaitForPrompt waits until the terminal window shows a shell
// prompt. Useful for either waiting for the startup process to finish
// or for a terminal application to exit.
func (ta *TerminalApp) WaitForPrompt() uiauto.Action {
	return ta.ui.WithTimeout(3 * time.Minute).WaitUntilExists(Prompt.Onscreen().First())
}

// ClickShelfMenuItem right clicks the terminal app icon on the shelf and left click the specified menu item.
func (ta *TerminalApp) ClickShelfMenuItem(itemNameRegexp string) uiauto.Action {
	return func(ctx context.Context) error {
		revert, err := ash.EnsureTabletModeEnabled(ctx, ta.tconn, false)
		if err != nil {
			testing.ContextLog(ctx, "Unable to switch out of tablet mode, try to swipe up the hot seat")
			tc, err := pointer.NewTouchController(ctx, ta.tconn)
			if err != nil {
				return errors.Wrap(err, "failed to create the touch controller")
			}
			defer tc.Close(ctx)
			if err := ash.SwipeUpHotseatAndWaitForCompletion(ctx, ta.tconn, tc.EventWriter(), tc.TouchCoordConverter()); err != nil {
				return errors.Wrap(err, "failed to swipe up the hotseat")
			}
		}
		if revert != nil {
			defer revert(ctx)
		}

		menuItem := nodewith.NameRegex(regexp.MustCompile(itemNameRegexp)).Role(role.MenuItem)
		return uiauto.Combine("click menu item on the Shelf",
			ta.ui.RightClickUntil(
				nodewith.Name("Terminal").Role(role.Button).First(),
				ta.ui.WithTimeout(time.Second).WaitUntilExists(menuItem)),
			ta.ui.LeftClick(menuItem),
		)(ctx)
	}
}

// LaunchThroughIcon launches Crostini by clicking the terminal app icon in launcher.
func LaunchThroughIcon(ctx context.Context, tconn *chrome.TestConn) (*TerminalApp, error) {
	// TODO(jinrongwu): type the whole name of Terminal instead of t when the following problem fixed.
	// The problem is: the launcher exits if typing more than one letter. This problem does not exists in other tests.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}
	if err := launcher.SearchAndLaunchWithQuery(tconn, kb, "t", "Terminal")(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to launch Terminal app")
	}

	ta, err := Find(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find Terminal window")
	}
	return ta, nil
}

// RestartCrostini shuts down Crostini and launch and exit the Terminal window.
func (ta *TerminalApp) RestartCrostini(keyboard *input.KeyboardEventWriter, cont *vm.Container, userName string) uiauto.Action {
	return func(ctx context.Context) error {
		if err := ta.ShutdownCrostini(cont)(ctx); err != nil {
			return errors.Wrap(err, "failed to shutdown crostini")
		}

		// Start the VM and container.
		ta, err := Launch(ctx, ta.tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch terminal")
		}

		if err := cont.Connect(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to connect to restarted container")
		}

		if err := ta.Close()(ctx); err != nil {
			return errors.Wrap(err, "failed to close Terminal app after restart")
		}

		return nil
	}
}

// ShutdownCrostini shuts down Crostini.
func (ta *TerminalApp) ShutdownCrostini(cont *vm.Container) uiauto.Action {
	return func(ctx context.Context) error {
		if err := ta.ClickShelfMenuItem("Shut down Linux")(ctx); err != nil {
			return errors.Wrap(err, "failed to shutdown crostini")
		}
		if err := ta.Close()(ctx); err != nil {
			return errors.Wrap(err, "failed to close Terminal app after shutdown")
		}

		err := testing.Poll(ctx, func(ctx context.Context) error {
			// While the VM is down, this command is expected to fail.
			if out, err := cont.Command(ctx, "pwd").Output(); err == nil {
				return errors.Errorf("expected command to fail while the container was shut down, but got: %q", string(out))
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second})
		if err != nil {
			return errors.Wrap(err, "VM failed to stop")
		}

		return nil
	}
}

// RunCommand runs command in Terminal windows.
func (ta *TerminalApp) RunCommand(keyboard *input.KeyboardEventWriter, cmd string) uiauto.Action {
	return uiauto.Combine("run command "+cmd,
		// Focus on the Terminal window.
		ta.ui.LeftClick(ta.tab),
		ta.ui.WaitUntilExists(terminalTextField.Focused().Editable()),
		// Type command.
		keyboard.TypeAction(cmd),
		// Press Enter.
		keyboard.AccelAction("Enter"))
}

// Exit closes the Terminal App through entering exit in the Terminal window.
func (ta *TerminalApp) Exit(keyboard *input.KeyboardEventWriter) uiauto.Action {
	return uiauto.Combine("exit Terminal window",
		ta.RunCommand(keyboard, "exit"),
		ta.ui.WithTimeout(time.Minute).WaitUntilGone(ta.tab),
		ta.Kb.AccelAction("Ctrl+Shift+W"))
}

// Close closes the Terminal App through clicking Close on shelf context menu.
func (ta *TerminalApp) Close() uiauto.Action {
	return uiauto.Combine("close Terminal window",
		ta.ClickShelfMenuItem("Close"),
		ta.dismissLeaveAppDialogIfShown(),
		ta.ui.WithTimeout(time.Minute).WaitUntilGone(rootWindow))
}

// CheckTabsCount returns an action to check the tabs count.
func (ta *TerminalApp) CheckTabsCount(nonTmuxTabs, tmuxTabs int) uiauto.Action {
	return func(ctx context.Context) error {
		// We use class name "WebContentsViewAura" instead of "Tab" because it has
		// the up-to-date title readily available.
		nodesInfo, err := ta.ui.NodesInfo(ctx, nodewith.ClassName("WebContentsViewAura"))
		if err != nil {
			return err
		}

		actualTmuxTabs := 0
		for _, n := range nodesInfo {
			if strings.HasPrefix(n.Name, "[tmux]") {
				actualTmuxTabs++
			}
		}

		if len(nodesInfo)-actualTmuxTabs != nonTmuxTabs {
			return errors.Errorf("number of normal tabs (%d) does not match expectation (%d)", len(nodesInfo)-actualTmuxTabs, nonTmuxTabs)
		}

		if actualTmuxTabs != tmuxTabs {
			return errors.Errorf("number of tmux tabs (%d) does not match expectation (%d)", actualTmuxTabs, tmuxTabs)
		}

		return nil
	}
}

// WaitForTabsCount returns an action to wait for the tabs count to match expectation.
func (ta *TerminalApp) WaitForTabsCount(nonTmuxTabs, tmuxTabs int) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, ta.CheckTabsCount(nonTmuxTabs, tmuxTabs), &testing.PollOptions{Timeout: 2 * time.Second})
	}
}

// ClickNthTabUntilNodeExists clicks the nth (0-index) tab.
func (ta *TerminalApp) ClickNthTabUntilNodeExists(n int, finder *nodewith.Finder) uiauto.Action {
	return ta.ui.LeftClickUntil(nodewith.Role(role.Tab).ClassName("Tab").Nth(n), ta.ui.WaitUntilExists(finder))
}

// ClickNthTabCloseButton clicks the nth (0-index) tab close button. Note that
// the home tab does not have this button, so you might need to adjust n
// accordingly.
func (ta *TerminalApp) ClickNthTabCloseButton(n int) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("clicking the tab close button (n=%d)", n),
		ta.ui.LeftClick(nodewith.ClassName("TabCloseButton").Nth(n)),
		ta.dismissLeaveAppDialogIfShown(),
	)
}

// Row returns a finder to match a row in the terminal screen.
func Row(content string) *nodewith.Finder {
	return AsRow(nodewith.Name(content))
}

// AsRow augments a finder to match a row in the terminal screen.
func AsRow(finder *nodewith.Finder) *nodewith.Finder {
	return finder.Role(role.StaticText).Ancestor(nodewith.ClassName("xterm-accessibility-tree"))
}

// LaunchTmux calls LaunchSSH first then launch tmux.
func LaunchTmux(ctx context.Context, tconn *chrome.TestConn) (ta *TerminalApp, retErr error) {
	ta, err := LaunchSSH(ctx, tconn, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open ssh")
	}

	defer func(ctx context.Context) {
		if retErr != nil {
			ta.Close()(ctx)
		}
	}(ctx)

	if err := uiauto.Combine("run tmux commands",
		ta.RunSSHCommand("tmux kill-server"),
		ta.RunSSHCommand("tmux -CC new -As test"),
		// Note that there is also an extra home tab.
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 1 /*tmuxTabs*/),
	)(ctx); err != nil {
		return nil, err
	}
	return ta, nil
}

// DontModifyPS1 creates the special file to not have the shell defaults modify
// PS1. This is useful for tests which depend on the prompt string containing
// "localhost".
func DontModifyPS1(ctx context.Context) (cleanUp func(), err error) {
	filePath := "/run/dont-modify-ps1-for-testing"
	err = os.WriteFile(filePath, []byte(""), 0644)
	if err != nil {
		testing.ContextLogf(ctx, "Error writing %s: %v", filePath, err)
		return func() {}, nil
	}

	cleanUp = func() {
		err := os.Remove(filePath)
		if err != nil {
			testing.ContextLogf(ctx, "Error unlinking %s: %v", filePath, err)
		}
	}

	return cleanUp, nil
}
