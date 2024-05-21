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

// BreakSWAAttractLoop waits until the Demo Session has started, and then exits
// out of the auto-launched Attract Loop screensaver by moving the mouse.
func BreakSWAAttractLoop(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(50 * time.Second)

	// Verify that splash screen has disappeared before moving mouse.
	splashScreen := nodewith.ClassName("WallpaperView").Ancestor(nodewith.ClassName("AlwaysOnTopWallpaperContainer"))
	if err := ui.WaitUntilGone(splashScreen)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until splash screen is gone")
	}

	demoApp := nodewith.Name("ChromeOS Highlights").First()
	if err := ui.WaitUntilExists(demoApp)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until Demo App exists")
	}

	pc := pointer.NewMouse(tconn)
	defer pc.Close(ctx)

	demoAppLocation, err := ui.Location(ctx, demoApp)
	if err != nil {
		return errors.Wrap(err, "failed to find location of Demo Mode App")
	}

	// Move mouse (arbitrarily) from center of demo app to screen corner to
	// trigger interaction, breaking fullscreen Attract Loop.
	if err := pc.Drag(
		demoAppLocation.CenterPoint(),
		pc.DragTo(coords.NewPoint(0, 0), 1*time.Second))(ctx); err != nil {
		return errors.Wrap(err, "failed to drag mouse across screen")
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
