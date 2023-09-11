// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HideContinueSectionTablet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the tablet launcher continue section can be hidden",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
	})
}

func HideContinueSectionTablet(ctx context.Context, s *testing.State) {
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
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Set up the launcher test. This enters tablet mode, which will show the
	// sorting nudge.
	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, true /*tabletMode*/, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	// Dismiss the sorting nudge.
	if err := launcher.DismissSortNudgeIfExists(ctx, tconn); err != nil {
		s.Fatal("Failed to dismiss sort nudge: ", err)
	}

	// Ensure continue section exists.
	ui := uiauto.New(tconn)
	continueSection := nodewith.ClassName("ContinueSectionView")
	if err := ui.WaitUntilExists(continueSection)(ctx); err != nil {
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

	// Select "Hide all suggestions" from the desktop context menu.
	if err := uiauto.Combine("Hide all suggestions",
		mouse.Click(tconn, coords.Point{X: 4, Y: 4}, mouse.RightButton),
		ui.LeftClick(nodewith.ClassName("MenuItemView").Name("Hide all suggestions")),
	)(ctx); err != nil {
		s.Fatal("Failed to select hide suggestions context menu item: ", err)
	}

	// Continue task should be hidden.
	if err := ui.WaitUntilGone(continueTask)(ctx); err != nil {
		s.Fatal("Failed to hide continue task: ", err)
	}

	// Select "Show all suggestions" from the desktop context menu.
	if err := uiauto.Combine("Show all suggestions",
		mouse.Click(tconn, coords.Point{X: 4, Y: 4}, mouse.RightButton),
		ui.LeftClick(nodewith.ClassName("MenuItemView").Name("Show all suggestions")),
	)(ctx); err != nil {
		s.Fatal("Failed to select show suggestions context menu item: ", err)
	}

	// Continue task should be visible again.
	if err := ui.WaitUntilExists(continueTask)(ctx); err != nil {
		s.Fatal("Failed to show continue task: ", err)
	}
}
