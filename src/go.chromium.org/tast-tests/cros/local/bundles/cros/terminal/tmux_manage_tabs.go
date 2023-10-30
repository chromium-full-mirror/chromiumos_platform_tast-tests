// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package terminal

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/terminalapp"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TmuxManageTabs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test management of tabs of Terminal app tmux integration",
		Contacts: []string{
			"guestos-ui@google.com",
			"lxj@google.com",
		},
		BugComponent: "b:658562",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		// It takes time to WaitForLocation for the last tab.
		// With the default 2min, it sometimes runs timeout.
		Timeout: 3 * time.Minute,
	})
}

func TmuxManageTabs(ctx context.Context, s *testing.State) {
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

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

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

	if err := uiauto.Combine("open two new tmux tabs and check list windows",
		ui.LeftClick(nodewith.ClassName("NewTabButton")),
		ui.WaitUntilExists(terminalapp.SSHPrompt),
		ui.LeftClick(nodewith.ClassName("NewTabButton")),
		ui.WaitUntilExists(terminalapp.SSHPrompt),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 3 /*tmuxTabs*/),
		// Switch back to the controlling tab.
		ta.ClickNthTabUntilNodeExists(1, terminalapp.TmuxModeMsg),
		ta.RunSSHCommand("list-windows -F 'tmux-window'"),
	)(ctx); err != nil {
		s.Fatal("Failed to open more tabs and check window number: ", err)
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
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		s.Fatal("Failed to check tmux window count: ", err)
	}

	if err := uiauto.Combine("close the first and the third tmux tab with the close button",
		// Note that the home tab does not have the close button, and we need to
		// adjust the index for that.
		ta.ClickNthTabCloseButton(3),
		// The size of the tabs change after a tab is closed.
		// Sleep to wait it become stable.
		uiauto.Sleep(2*time.Second),
		ta.ClickNthTabCloseButton(1),
		uiauto.Sleep(2*time.Second),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 1 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to close tabs: ", err)
	}

	tabs := nodewith.Role(role.Tab).ClassName("Tab")
	nodes, _ := ui.NodesInfo(ctx, tabs)
	last := len(nodes) - 1
	if err := uiauto.Combine("close the only remaining tmux tab",
		ui.WithTimeout(30*time.Second).WaitForLocation(tabs.Nth(last)),
		ta.ClickNthTabUntilNodeExists(last, terminalapp.SSHPrompt),
		ta.RunSSHCommand("exit"),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 0 /*tmuxTabs*/),
		// The controlling tab should be in focus now. We want to check that the
		// integration mode has exited. We use regex here because the beginning of
		// the prompt might be polluted with the tmux prompt ">>> ".
		ui.WaitUntilExists(terminalapp.AsRow(nodewith.NameRegex(regexp.MustCompile(`chronos@localhost ~ \$\s*$`)))),
	)(ctx); err != nil {
		s.Fatal("Failed: ", err)
	}
}
