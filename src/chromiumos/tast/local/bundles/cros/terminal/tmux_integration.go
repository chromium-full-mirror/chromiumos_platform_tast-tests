// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package terminal

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/crostini/ui/terminalapp"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TmuxIntegration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify Terminal app tmux integration is working fine",
		Contacts: []string{
			"guestos-ui@google.com",
			"lxj@chromium.org",
		},
		BugComponent: "b:658562",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

func TmuxIntegration(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("TerminalAlternativeEmulator", "TerminalTmuxIntegration"))
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Get Test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	ui := uiauto.New(tconn)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ta, err := terminalapp.LaunchSSH(ctx, tconn, "")
	if err != nil {
		s.Fatal("Failed to open ssh: ", err)
	}

	if err := uiauto.Combine("run tmux commands",
		ta.RunSSHCommand("tmux kill-server"),
		ta.RunSSHCommand("tmux -CC new -As test"),
		// Note that there is also an extra home tab.
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 1 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to run tmux commands: ", err)
	}

	if err := ta.ClickNthTabUntilNodeExists(1, terminalapp.TmuxModeMsg)(ctx); err != nil {
		s.Fatal("Failed to verify controlling tab: ", err)
	}

	echoContent := "hello world"
	vimContent := "abcdefg"

	if err := uiauto.Combine("interact with the first tmux tab",
		ta.ClickNthTabUntilNodeExists(2, terminalapp.CmdPrompt),
		ta.RunSSHCommand("echo "+echoContent),
		ui.WaitUntilExists(terminalapp.Row(echoContent)),
	)(ctx); err != nil {
		s.Fatal("Failed to interact with the first tmux tab: ", err)
	}

	if err := uiauto.Combine("open new tmux tab",
		ui.LeftClick(nodewith.ClassName("NewTabButton")),
		ui.WaitUntilExists(terminalapp.CmdPrompt),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 2 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to open new tmux tab: ", err)
	}

	if err := uiauto.Combine("run vim in the second tmux tab",
		ta.RunSSHCommand("rm -f /tmp/tmux_integration_test"),
		ta.RunSSHCommand("vim -nu NONE /tmp/tmux_integration_test"),
		// Wait for vim to run.
		ui.WaitUntilExists(terminalapp.AsRow(nodewith.NameContaining("[New File]").First())),
		ta.Kb.TypeAction("i"),
		ta.Kb.TypeAction(vimContent),
		ta.Kb.AccelAction("Esc"),
		ui.WaitUntilExists(terminalapp.Row(vimContent)),
		ta.Kb.TypeAction(":w"),
		ta.Kb.AccelAction("Enter"),
		// Put vim into background.
		ta.Kb.AccelAction("Ctrl+Z"),
		ui.WaitUntilExists(terminalapp.CmdPrompt),
		ta.RunSSHCommand("echo -n 'content: ' && cat /tmp/tmux_integration_test"),
		ui.WaitUntilExists(terminalapp.Row("content: "+vimContent)),
		// Bring vim back to foreground.
		ta.RunSSHCommand("fg"),
		ui.WaitUntilExists(terminalapp.Row(vimContent)),
	)(ctx); err != nil {
		s.Fatal("Failed to run vim in the second tmux tab: ", err)
	}

	if err := uiauto.Combine("detach the tmux session",
		ta.ClickNthTabUntilNodeExists(1, terminalapp.TmuxModeMsg),
		ta.Kb.AccelAction("Ctrl+C"),
		ui.WaitUntilExists(terminalapp.CmdPrompt),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 0 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to detach the tmux session: ", err)
	}

	if err := uiauto.Combine("reattach the tmux session",
		ta.RunSSHCommand("tmux -CC new -As test"),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 2 /*tmuxTabs*/),
		// Check first tmux tab.
		ta.ClickNthTabUntilNodeExists(2, terminalapp.Row(echoContent)),
		// Check second tmux tab.
		ta.ClickNthTabUntilNodeExists(3, terminalapp.Row(vimContent)),
	)(ctx); err != nil {
		s.Fatal("Failed to reattach the tmux session: ", err)
	}

	if err := uiauto.Combine("detach the tmux session by closing the controlling tab",
		// Note that the home tab does not have the close button, so we use 0 here.
		ta.ClickNthTabCloseButton(0),
		ta.WaitForTabsCount(1 /*nonTmuxTabs*/, 0 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to detach the tmux session by closing the controlling tab: ", err)
	}

	if err := uiauto.Combine("open ssh and reattach the tmux session",
		ta.OpenSSHConnection(),
		ta.RunSSHCommand("tmux -CC new -As test"),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 2 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to open ssh and reattach the tmux session: ", err)
	}

	if err := uiauto.Combine("open one more tmux tab and check tmux window count with list-window",
		ui.LeftClick(nodewith.ClassName("NewTabButton")),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 3 /*tmuxTabs*/),
		// Switch back to the controlling tab.
		ta.ClickNthTabUntilNodeExists(1, terminalapp.TmuxModeMsg),
		ta.RunSSHCommand("list-windows -F 'tmux-window'"),
	)(ctx); err != nil {
		s.Fatal("Failed: ", err)
	}

	// Check the number of tmux windows in the result for "list-windows".
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		nodesInfo, err := ui.NodesInfo(ctx, terminalapp.Row("tmux-window"))
		if err != nil {
			return testing.PollBreak(err)
		}
		if len(nodesInfo) != 3 {
			return errors.Errorf("Tmux window count (%d) does not equal 3", len(nodesInfo))
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Second}); err != nil {
		s.Fatal("Failed to check tmux window count: ", err)
	}

	if err := uiauto.Combine("close the first and the third tmux tab with the close button",
		// Note that the home tab does not have the close button, and we need to
		// adjust the index for that.
		ta.ClickNthTabCloseButton(3),
		ta.ClickNthTabCloseButton(1),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 1 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed: ", err)
	}

	if err := uiauto.Combine("close the only remaining tmux tab",
		ta.ClickNthTabUntilNodeExists(2, terminalapp.Row(vimContent)),
		// The remaining one is running vim. Let exit it cleanly.
		ta.Kb.TypeAction(":qa!"),
		ta.Kb.AccelAction("Enter"),
		ui.WaitUntilExists(terminalapp.CmdPrompt),
		ta.RunSSHCommand("exit"),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 0 /*tmuxTabs*/),
		// The controlling tab should be in focus now. We want to check that the
		// integration mode has exited. We use regex here because the beginning of
		// the prompt might be polluted with the tmux prompt ">>> ".
		ui.WaitUntilExists(terminalapp.AsRow(nodewith.NameRegex(regexp.MustCompile(`chronos@localhost ~ \$\s*$`)))),
	)(ctx); err != nil {
		s.Fatal("Failed: ", err)
	}

	if err := ta.Close()(ctx); err != nil {
		s.Fatal("Failed to close terminal window: ", err)
	}
}
