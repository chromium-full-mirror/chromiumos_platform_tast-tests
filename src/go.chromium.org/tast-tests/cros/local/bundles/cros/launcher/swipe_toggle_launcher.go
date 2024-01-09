// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SwipeToggleLauncher,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the swipe gesture to show and hide bubble launcher",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350", // ChromeOS > Software > System UI Surfaces > Launcher
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.TouchScreen()),
		Timeout:      3*time.Minute + cws.InstallationTimeout,
	})
}

func SwipeToggleLauncher(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	opt := chrome.EnableFeatures("QuickActionShowBubbleLauncher")

	// Start a new chrome session to avoid reusing user sessions and verify that the privacy nudge gets shown.
	cr, err := chrome.New(ctx, opt)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	var tsew *input.TouchscreenEventWriter
	var tcc *input.TouchCoordConverter
	var stw *input.SingleTouchEventWriter

	tsew, tcc, err = touch.NewTouchscreenAndConverter(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to access to the touchscreen: ", err)
	}
	defer tsew.Close(ctx)

	stw, err = tsew.NewSingleTouchWriter()
	if err != nil {
		s.Fatal("Failed to create the single touch writer: ", err)
	}

	// Make sure the shelf bounds is stable before swiping.
	if err := ash.WaitForStableShelfBounds(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for stable shelf bounds: ", err)
	}

	homeButton, err := ui.Location(ctx, launcher.HomeButtonFinder)
	if err != nil {
		s.Fatal("Failed to get locaton for home button: ", err)
	}

	// Swipe up from the home button should show the bubble launcher.
	if err := performSwipe(ctx, homeButton.BottomCenter(), homeButton.TopCenter(), tcc, stw); err != nil {
		s.Fatal("Failed to swipe up on home button: ", err)
	}

	bubbleLauncher := nodewith.ClassName("AppListBubbleView")

	if err := ui.WaitUntilExists(bubbleLauncher)(ctx); err != nil {
		s.Fatal("Failed to wait for bubble launcher show: ", err)
	}

	// Swiping down on the home button with the bubble launncher showing should hide the launcher.
	if err := performSwipe(ctx, homeButton.TopCenter(), homeButton.BottomCenter(), tcc, stw); err != nil {
		s.Fatal("Failed to swipe down on home button: ", err)
	}

	if err := ui.WaitUntilGone(bubbleLauncher)(ctx); err != nil {
		s.Fatal("Failed to wait for bubble launcher: ", err)
	}

	startStatusArea, err := ui.Location(ctx, nodewith.HasClass("UnifiedSystemTray").First())
	startStatusAreaInset := startStatusArea.WithInset(1, 1)

	// Swipe on the status area should not show the bubble launcher.
	if err := performSwipe(ctx, startStatusAreaInset.BottomCenter(), startStatusAreaInset.TopCenter(), tcc, stw); err != nil {
		s.Fatal("Failed to swipe up on status area: ", err)
	}

	if err := ui.EnsureGoneFor(bubbleLauncher, 10*time.Second)(ctx); err != nil {
		s.Fatal("The launcher was shown: ", err)
	}

}

func performSwipe(ctx context.Context, start, end coords.Point, tcc *input.TouchCoordConverter, stw *input.SingleTouchEventWriter) error {
	swipeDuration := 200 * time.Millisecond
	startX, startY := tcc.ConvertLocation(start)
	endX, endY := tcc.ConvertLocation(end)

	if err := stw.Swipe(ctx, startX, startY, endX, endY, swipeDuration); err != nil {
		return errors.Wrap(err, "failed to swipe up")
	}
	if err := stw.End(); err != nil {
		return errors.Wrap(err, "failed to finish the swipe gesture")
	}

	return nil

}
