// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HideContinueSectionClamshell,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the clamshell launcher continue section can be hidden",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		SoftwareDeps: []string{"chrome"},
	})
}

func HideContinueSectionClamshell(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	opt := chrome.EnableFeatures(
		"ProductivityLauncher:enable_continue/true", // Enable continue section
		"ForceShowContinueSection")                  // Populate continue section with items
	cr, err := chrome.New(ctx, opt)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Bubble launcher requires clamshell mode.
	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Dismiss the sort nudge. This nudge is shown when the user opens the
	// launcher for the first time. If the nudge is visible, the continue
	// section privacy notice (which needs to be acknowledged before showing the
	// continue section) will be delayed until the sort nudge is accepted.
	// Generally, continue section privacy notice has higher precedence, but if
	// there's a delay when retrieving zero state results, sort nudge may be
	// shown before zero state results are returned. In this case, sort nudge
	// needs to be dismissed in order for continue section to show up.
	if err := launcher.DismissSortNudgeIfExists(ctx, tconn); err != nil {
		s.Fatal("Failed to dismiss sort nudge: ", err)
	}

	if err := uiauto.Combine("close and reopen bubble launcher",
		launcher.CloseBubbleLauncher(tconn),
		launcher.OpenBubbleLauncher(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to close and reopen the launcher: ", err)
	}

	// Ensure continue section exists.
	ui := uiauto.New(tconn)
	continueSection := nodewith.ClassName("ContinueSectionView")
	if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(continueSection)(ctx); err != nil {
		s.Fatal("Failed to show continue section: ", err)
	}

	// Dismiss the privacy notice.
	if err := launcher.DismissPrivacyNotice(ctx, tconn); err != nil {
		s.Fatal("Failed to dismiss privacy notice: ", err)
	}

	// Ensure at least one continue task is visible.
	continueTask := nodewith.Ancestor(continueSection).ClassName("ContinueTaskView").First()
	if err := ui.WaitUntilExists(continueTask)(ctx); err != nil {
		s.Fatal("Failed to find continue tasks: ", err)
	}

	// Clicking the "Hide all suggestions" button should hide continue tasks.
	bubble := nodewith.ClassName(ash.AppListBubbleClassName)
	hideButton := nodewith.Ancestor(bubble).Name("Hide all suggestions")
	if err := uiauto.Combine("Click hide all suggestions",
		ui.WaitUntilExists(hideButton),
		ui.LeftClick(hideButton),
		ui.WaitUntilGone(continueTask),
	)(ctx); err != nil {
		s.Fatal("Failed to hide continue tasks by clicking hide button: ", err)
	}

	// Clicking the "Show all suggestions" button should show continue tasks.
	showButton := nodewith.Ancestor(bubble).Name("Show all suggestions")
	if err := uiauto.Combine("Click show all suggestions",
		ui.WaitUntilExists(showButton),
		ui.LeftClick(showButton),
		ui.WaitUntilExists(continueTask),
	)(ctx); err != nil {
		s.Fatal("Failed to show continue tasks by clicking show button: ", err)
	}

	if err := launcher.CloseBubbleLauncher(tconn)(ctx); err != nil {
		s.Fatal("Failed to close bubble launcher: ", err)
	}
}
