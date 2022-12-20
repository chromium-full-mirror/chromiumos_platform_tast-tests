// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/pointer"
	"chromiumos/tast/local/coords"
)

// BreakSWAAttractLoop waits until the Demo Session has started, and then exits
// out of the auto-launched Attract Loop screensaver by moving the mouse.
//
// This method assumes the System Web App version of the demo apps is running
// (i.e the "DemoModeSWA" feature is enabled).
func BreakSWAAttractLoop(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(50 * time.Second)

	// Verify that splash screen has disappeared before moving mouse.
	splashScreen := nodewith.ClassName("WallpaperView").Ancestor(nodewith.ClassName("AlwaysOnTopWallpaperContainer"))
	if err := ui.WaitUntilGone(splashScreen)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until splash screen is gone")
	}

	demoApp := nodewith.Name("Demo Mode App").First()
	if err := ui.WaitUntilExists(demoApp)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until Demo App exists")
	}

	pc := pointer.NewMouse(tconn)
	defer pc.Close()

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
