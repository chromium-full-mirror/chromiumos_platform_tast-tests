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
		Func:         TmuxCloseControllingTab,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify closing the controlling tab in Terminal app tmux integration",
		Contacts: []string{
			"guestos-ui@google.com",
			"lxj@google.com",
		},
		BugComponent: "b:658562",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

func TmuxCloseControllingTab(ctx context.Context, s *testing.State) {
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

	if err := uiauto.Combine("open new tmux tab",
		ui.LeftClick(nodewith.ClassName("NewTabButton")),
		ui.WaitUntilExists(terminalapp.SSHPrompt),
		ta.WaitForTabsCount(2 /*nonTmuxTabs*/, 2 /*tmuxTabs*/),
	)(ctx); err != nil {
		s.Fatal("Failed to open new tmux tab: ", err)
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
}
