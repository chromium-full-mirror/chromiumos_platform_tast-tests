// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// WaitForDemoModeApp waits for the Demo Session to start, splash screen to be
// removed, and demo mode app to appear.
func WaitForDemoModeApp(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(50 * time.Second)

	// Verify that splash screen has disappeared before moving mouse.
	splashScreen := nodewith.ClassName("WallpaperView").Ancestor(nodewith.ClassName("AlwaysOnTopWallpaperContainer"))
	if err := ui.WaitUntilGone(splashScreen)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until splash screen is gone")
	}

	demoApp := nodewith.Name("ChromeOS Highlights").First()
	if err := ui.WaitUntilExists(demoApp)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until Demo Mode App exists")
	}
	return nil
}

// BreakSWAAttractLoop waits for the attract loop to appear in fullscreen, and
// then breaks it by clicking on the screen.
func BreakSWAAttractLoop(ctx context.Context, tconn *chrome.TestConn) error {
	pc := pointer.NewMouse(tconn)
	defer pc.Close(ctx)

	// Wait for Demo Mode app enters fullscreen after the attract loop starts to play.
	if err := ash.WaitForFullScreen(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait for Demo Mode app in fullscreen")
	}

	// Click mouse somewhere on the attract loop to trigger interaction, breaking
	// the fullscreen attract loop.
	if err := pc.ClickAt(
		coords.NewPoint(100, 100))(ctx); err != nil {
		return errors.Wrap(err, "failed to click mouse on the attract loop")
	}
	return nil
}

// VerifySWAFunctionality tests core SWA functionality (e.g. entering and exiting fullscreen,
// asserting that basic content has rendered). Many different customized versions of the SWA
// can be running on different devices, so highlightsNode is a node representing a UI element
// unique to the version of the Highlights App being tested, to verify that the proper version
// of the app is running.
func VerifySWAFunctionality(ctx context.Context, tconn *chrome.TestConn, highlightsNode,
	attractLoopNode *nodewith.Finder) error {
	ui := uiauto.New(tconn).WithTimeout(100 * time.Second)

	// Verify that splash screen has disappeared before moving mouse.
	splashScreen := nodewith.ClassName("WallpaperView").Ancestor(nodewith.ClassName("AlwaysOnTopWallpaperContainer"))
	if err := ui.WaitUntilGone(splashScreen)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until splash screen is gone")
	}

	testing.ContextLog(ctx, "Waiting for Demo Mode App to launch")
	demoApp := nodewith.Name("ChromeOS Highlights").First()
	if err := ui.WaitUntilExists(demoApp)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until Demo App exists")
	}

	testing.ContextLog(ctx, "Confirming that app is in fullscreen Attract Loop mode")
	if err := ash.WaitForFullscreenConditionWithTitle(tconn, "ChromeOS Highlights", true, 10*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for a window in fullscreen mode")
	}

	if err := ui.WaitUntilExists(attractLoopNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until attract loop exists")
	}

	// GoBigSleepLint: attractLoopNode` exists does not mean the video become stable. There
	// isn't any UI elements or conditions to wait for with the Test API `tconn` so sleep for
	// 6s now. If the video not get loaded after 6s, we should investigate this issue.
	if err := testing.Sleep(ctx, 6*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	if err := ui.LeftClick(attractLoopNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on attract loop")
	}

	testing.ContextLog(ctx, "Confirming that app is in windowed Highlights mode")
	if err := ash.WaitForFullscreenConditionWithTitle(tconn, "ChromeOS Highlights", false, 10*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for a window in non-fullscreen mode")
	}
	// Confirm that basic Highlights content is shown by presence of highlightsNode
	if err := ui.WaitUntilExists(highlightsNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for Highlights content node to be present")
	}
	return nil
}
