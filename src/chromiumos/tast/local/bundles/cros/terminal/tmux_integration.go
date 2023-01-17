// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package terminal

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
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
		BugComponent: "b:1122570",
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

	waitTmuxPromptMessage := ui.WaitUntilExists(nodewith.Name("Tmux integration mode activated. Press Ctrl-C to detach tmux, or input tmux commands").Role(role.StaticText))

	if err := uiauto.Combine("verify controlling tab",
		ta.ClickNthTab(1),
		waitTmuxPromptMessage,
	)(ctx); err != nil {
		s.Fatal("Failed to verify controlling tab: ", err)
	}

	echoContent := "hello world"
	vimContent := "abcdefg"

	if err := uiauto.Combine("interact with the first tmux tab",
		ta.ClickNthTab(2),
		ta.RunSSHCommand("echo "+echoContent),
		ui.WaitUntilExists(nodewith.Name(echoContent).Role(role.StaticText).First()),
	)(ctx); err != nil {
		s.Fatal("Failed to interact with the first tmux tab: ", err)
	}

	if err := uiauto.Combine("open new tmux tab",
		ui.LeftClick(nodewith.ClassName("NewTabButton")),
		ui.WaitUntilExists(nodewith.Name("chronos@localhost ~ $").Role(role.StaticText).First()),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 2 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to open new tmux tab: ", err)
	}

	if err := uiauto.Combine("run vim in the second tmux tab",
		ta.RunSSHCommand("rm -f /tmp/tmux_integration_test"),
		ta.RunSSHCommand("vim -nu NONE /tmp/tmux_integration_test"),
		// Wait for vim to run.
		ui.WaitUntilExists(nodewith.NameContaining("[New File]").Role(role.StaticText).First()),
		ta.Kb.TypeAction("i"),
		ta.Kb.TypeAction(vimContent),
		ta.Kb.AccelAction("Esc"),
		ui.WaitUntilExists(nodewith.Name(vimContent).Role(role.StaticText)),
		ta.Kb.TypeAction(":w"),
		ta.Kb.AccelAction("Enter"),
		// Put vim into background.
		ta.Kb.AccelAction("Ctrl+Z"),
		ui.WaitUntilExists(nodewith.Name("chronos@localhost ~ $").Role(role.StaticText).First()),
		ta.RunSSHCommand("echo -n 'content: ' && cat /tmp/tmux_integration_test"),
		ui.WaitUntilExists(nodewith.Name("content: "+vimContent).Role(role.StaticText)),
		// Bring vim back to foreground.
		ta.RunSSHCommand("fg"),
		ui.WaitUntilExists(nodewith.Name(vimContent).Role(role.StaticText)),
	)(ctx); err != nil {
		s.Fatal("Failed to run vim in the second tmux tab: ", err)
	}

	if err := uiauto.Combine("detach the tmux session",
		ta.ClickNthTab(1),
		waitTmuxPromptMessage,
		ta.Kb.AccelAction("Ctrl+C"),
		ui.WaitUntilExists(nodewith.Name("chronos@localhost ~ $").Role(role.StaticText).First()),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 0 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to detach the tmux session: ", err)
	}

	if err := uiauto.Combine("reattach the tmux session",
		ta.RunSSHCommand("tmux -CC new -As test"),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 2 /*tmuxTabs*/),
		// Check first tmux tab.
		ta.ClickNthTab(2),
		ui.WaitUntilExists(nodewith.Name("chronos@localhost ~ $ echo "+echoContent).Role(role.StaticText)),
		ui.Exists(nodewith.Name(echoContent).Role(role.StaticText)),
		// Check second tmux tab.
		ta.ClickNthTab(3),
		ui.WaitUntilExists(nodewith.Name(vimContent).Role(role.StaticText)),
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

	if err := ta.Close()(ctx); err != nil {
		s.Fatal("Failed to close terminal window: ", err)
	}
}
