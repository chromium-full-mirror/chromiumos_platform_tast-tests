// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package terminal

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/terminalapp"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TmuxBasicOperations,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify controlling tab, echo strings in the first Tmux tab, and detach and reattach tabs",
		Contacts: []string{
			"guestos-ui@google.com",
			"lxj@chromium.org",
		},
		BugComponent: "b:658562",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

func TmuxBasicOperations(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cleanup, err := terminalapp.DontModifyPS1(ctx)
	if err != nil {
		s.Fatal("Failed to set DontModifyPS1: ", err)
	}
	defer cleanup()

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

	var ta *terminalapp.TerminalApp
	defer func() {
		faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

		if ta != nil {
			ta.Close()(cleanupCtx)
		}
	}()

	if ta, err = terminalapp.LaunchTmux(ctx, tconn); err != nil {
		s.Fatal("Failed to launch Tmux: ", err)
	}

	// Verify the controlling tab.
	if err := ta.ClickNthTabUntilNodeExists(1, terminalapp.TmuxModeMsg)(ctx); err != nil {
		s.Fatal("Failed to verify controlling tab: ", err)
	}

	echoContent := "hello world"
	vimContent := "abcdefg"

	if err := uiauto.Combine("run simple command in the first tab",
		ta.ClickNthTabUntilNodeExists(2, terminalapp.CmdPrompt),
		ta.RunSSHCommand("echo "+echoContent),
		ui.WaitUntilExists(terminalapp.Row(echoContent)),
	)(ctx); err != nil {
		s.Fatal("Failed to test the first tmux tab: ", err)
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
}
