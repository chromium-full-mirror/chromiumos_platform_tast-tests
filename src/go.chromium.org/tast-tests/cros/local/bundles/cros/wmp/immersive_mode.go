// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wmp/wmputils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ImmersiveMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that immersive mode works correctly",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Immersive
		BugComponent: "b:1252580",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func ImmersiveMode(ctx context.Context, s *testing.State) {
	const (
		timeout = 30 * time.Second
	)

	// Shorten context for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(closeCtx)

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open: ", err)
	}

	// Open a browser window either ash-chrome or lacros-chrome.
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find browser app info: ", err)
	}
	if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
		s.Fatal("Failed to launch chrome: ", err)
	}

	// Ensure that there is only one open window that is the primary browser. Wait for the browser to be visible to avoid a race that may cause test flakiness.
	bt := browser.TypeAsh
	bw, err := wmputils.EnsureOnlyBrowserWindowOpen(ctx, tconn, bt)
	if err != nil {
		s.Fatal("Failed to ensure one browser window: ", err)
	}
	defer bw.CloseWindow(closeCtx, tconn)

	// Press the zoom toggle key to trigger immersive mode.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Not all Chromebooks have the same layout for the function keys.
	layout, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		s.Fatal("Failed to get keyboard mapping: ", err)
	}

	if err = kb.Accel(ctx, layout.ZoomToggle); err != nil {
		s.Fatal("Failed to press the immersive mode shortcut: ", err)
	}

	// Check the chrome window is in immersive mode.
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == bw.ID && w.State == ash.WindowStateFullscreen && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second}); err != nil {
		s.Fatalf("Expected the window to be fullscreen but it is %s", bw.State)
	}

	// Launcher should be hidden in immersive mode.
	ac := uiauto.New(tconn).WithTimeout(timeout)
	if err := ac.WaitUntilGone(launcher.HomeButtonFinder)(ctx); err != nil {
		s.Fatal("Launcher button is present in immersive mode: ", err)
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}
	// Swipe up from the bottom edge to the center.
	tc, err := touch.New(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the touch context: ", err)
	}
	screenBottomPt := info.Bounds.BottomCenter().Sub(coords.NewPoint(0, 1))
	screenCenterPt := info.Bounds.CenterPoint()
	if err := uiauto.Combine(
		"swipe up from the bottom edge, and check if the launcher appears",
		tc.Swipe(screenBottomPt, tc.SwipeTo(screenCenterPt, time.Second)),
		// Launcher button should appear after the swipe.
		ac.WaitUntilExists(launcher.HomeButtonFinder),
	)(ctx); err != nil {
		s.Fatal("Failed to swipe up to reveal launcher: ", err)
	}
}
