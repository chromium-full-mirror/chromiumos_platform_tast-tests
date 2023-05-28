// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package windowarrangementcuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// multiresize summons a multiresizer and drags it like dragAndRestore, but
// with all drag points adjusted for the location of the multiresizer.
func multiresize(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context, duration time.Duration, dragPoints ...coords.Point) error {
	// Move the mouse near the first drag point until the multiresize widget appears.
	multiresizer := nodewith.Role("window").ClassName("MultiWindowResizeController")
	for hoverOffset := -5; ; hoverOffset++ {
		if err := mouse.Move(tconn, dragPoints[0].Add(coords.NewPoint(hoverOffset, hoverOffset)), 100*time.Millisecond)(ctx); err != nil {
			return errors.Wrap(err, "failed to move mouse")
		}

		multiresizerExists, err := ui.IsNodeFound(ctx, multiresizer)
		if err != nil {
			return errors.Wrap(err, "failed to check for multiresizer")
		}
		if multiresizerExists {
			break
		}

		if hoverOffset == 5 {
			return errors.New("never found multiresize widget")
		}
	}

	multiresizerBounds, err := ui.ImmediateLocation(ctx, multiresizer)
	if err != nil {
		return errors.Wrap(err, "failed to get the multiresizer location")
	}

	offset := multiresizerBounds.CenterPoint().Sub(dragPoints[0])
	var multiresizeDragPoints []coords.Point
	for _, p := range dragPoints {
		multiresizeDragPoints = append(multiresizeDragPoints, p.Add(offset))
	}

	if err := dragAndRestore(ctx, tconn, pc, duration, multiresizeDragPoints...); err != nil {
		return errors.Wrap(err, "failed to drag multiresizer")
	}

	return nil
}

// RunClamShell runs window arrangement cuj for clamshell. We test performance
// for resizing window, dragging window, maximizing window, minimizing window
// and split view resizing.
func RunClamShell(ctx, closeCtx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context) (retErr error) {
	const (
		timeout  = 10 * time.Second
		duration = 2 * time.Second
	)

	// Gets primary display info and interesting drag points.
	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the primary display info")
	}
	splitViewDragPoints, snapLeftPoint, snapRightPoint := getSplitViewDragPoints(info)

	// Get the browser window.
	ws, err := getAllNonPipWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the window list")
	}
	if len(ws) != 1 {
		return errors.Errorf("unexpected number of windows: got %d, want 1", len(ws))
	}
	browserWinID := ws[0].ID

	// Set the browser window state to "Normal".
	if err := ash.SetWindowStateAndWait(ctx, tconn, browserWinID, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "failed to set browser window state to \"Normal\"")
	}

	// Initialize the browser window bounds.
	desiredBounds := info.WorkArea.WithInset(50, 50)
	bounds, displayID, err := ash.SetWindowBounds(ctx, tconn, browserWinID, desiredBounds, info.ID)
	if err != nil {
		return errors.Wrap(err, "failed to set the browser window bounds")
	}
	if displayID != info.ID {
		return errors.Errorf("unexpected display ID for browser window: got %q; want %q", displayID, info.ID)
	}
	if bounds != desiredBounds {
		return errors.Errorf("unexpected browser window bounds: got %v; want %v", bounds, desiredBounds)
	}

	// Wait for the browser window to finish animating to the desired bounds.
	if err := ash.WaitWindowFinishAnimating(ctx, tconn, browserWinID); err != nil {
		return errors.Wrap(err, "failed to wait for the browser window animation")
	}

	// GoBigSleepLint: Wait a little longer because the following resizing drag has
	// had several flaky problems which do not cause test failures
	// but lead to misleading performance data. See b/263131909.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	// Resize window.
	upperLeftPt := coords.NewPoint(bounds.Left, bounds.Top)
	middlePt := coords.NewPoint(bounds.Left+bounds.Width/2, bounds.Top+bounds.Height/2)
	testing.ContextLog(ctx, "Resizing the browser window")
	if err := dragAndRestore(ctx, tconn, pc, duration, upperLeftPt, middlePt); err != nil {
		return errors.Wrap(err, "failed to resize browser window from the upper left to the middle and back")
	}

	// Drag window.
	newTabButton := nodewith.Name("New Tab").HasClass("NewTabButton").Role(role.Button)
	newTabButtonRect, err := ui.Location(ctx, newTabButton)
	if err != nil {
		return errors.Wrap(err, "failed to get the location of the new tab button")
	}
	tabStripGapPt := coords.NewPoint(newTabButtonRect.Right()+10, newTabButtonRect.Top)
	testing.ContextLog(ctx, "Dragging the browser window")
	if err := dragAndRestore(ctx, tconn, pc, duration, tabStripGapPt, middlePt); err != nil {
		return errors.Wrap(err, "failed to drag browser window from the tab strip point to the middle and back")
	}

	// Maximize window.
	maximizeButton := nodewith.Name("Maximize").HasClass("FrameSizeButton").Role(role.Button)
	if err := pc.Click(maximizeButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to maximize the browser window")
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == browserWinID && w.State == ash.WindowStateMaximized && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed to wait for browser window to become maximized")
	}

	// Minimize window.
	minimizeButton := nodewith.Name("Minimize").HasClass("FrameCaptionButton").Role(role.Button)
	if err := pc.Click(minimizeButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to minimize the browser window")
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == browserWinID && w.State == ash.WindowStateMinimized && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed to wait for browser window to become minimized")
	}

	// Unminimize window.
	chromeAppIcon := nodewith.NameContaining("Chrome").HasClass("ash/ShelfAppButton").Role(role.Button)
	if err := pc.Click(chromeAppIcon)(ctx); err != nil {
		return errors.Wrap(err, "failed to unminimize the browser window")
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == browserWinID && w.State == ash.WindowStateMaximized && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed to wait for browser window to be unminimized")
	}

	// Browser window is now maximized so calculate a new grab point.
	newBrowserWin, err := ash.GetWindow(ctx, tconn, browserWinID)
	if err != nil {
		return errors.Wrap(err, "failed to get browser window info")
	}
	newBounds := newBrowserWin.BoundsInRoot
	tabStripGapPt = coords.NewPoint(newBounds.Left+newBounds.Width*3/4, newBounds.Top+10)

	// Snap the window to the left and drag the second tab to snap to the right.
	testing.ContextLog(ctx, "Snapping the browser window to the left")
	if err := pc.Drag(
		tabStripGapPt,
		uiauto.Sleep(dragStartWaitTime),
		pc.DragTo(snapLeftPoint, duration),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to snap the browser window to the left")
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.ID == browserWinID && w.State == ash.WindowStatePrimarySnapped && !w.IsAnimating
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed to wait for browser window to be left snapped")
	}
	testing.ContextLog(ctx, "Snapping the second tab to the right")
	firstTab := nodewith.Role(role.Tab).ClassName("Tab").First()
	firstTabRect, err := ui.Location(ctx, firstTab)
	if err != nil {
		return errors.Wrap(err, "failed to get the location of the first tab")
	}
	if err := pc.Drag(
		firstTabRect.CenterPoint(),
		uiauto.Sleep(dragStartWaitTime),
		pc.DragTo(snapRightPoint, duration),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to snap the second tab to the right")
	}
	defer cleanUp(ctx, action.Named(
		"recombine the browser tabs",
		func(ctx context.Context) error {
			return combineTabs(ctx, tconn, ui, pc, duration)
		},
	), &retErr)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ws, err := getAllNonPipWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to obtain the window list")
		}
		if len(ws) != 2 {
			return errors.Errorf("should be 2 windows, got %v", len(ws))
		}
		if (ws[1].State == ash.WindowStatePrimarySnapped && ws[0].State == ash.WindowStateSecondarySnapped) ||
			(ws[0].State == ash.WindowStatePrimarySnapped && ws[1].State == ash.WindowStateSecondarySnapped) {
			return nil
		}
		return errors.New("browser windows are not snapped yet")
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return errors.Wrap(err, "failed to wait for browser windows to be snapped correctly")
	}

	// Use multiresize on the two snapped windows.
	testing.ContextLog(ctx, "Multiresizing two snapped browser windows")
	const dividerDragError = "failed to drag divider slightly left, all the way right, and back to center"
	if err := multiresize(ctx, tconn, ui, pc, duration, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, dividerDragError)
	}

	// Enter the overview mode.
	if err := setOverviewModeAndWait(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to enter overview mode")
	}
	defer cleanUp(closeCtx, action.Named(
		"ensure not in overview",
		func(ctx context.Context) error {
			return ash.SetOverviewModeAndWait(ctx, tconn, false)
		},
	), &retErr)
	// Create a second virtual desk.
	if err := ash.CreateNewDesk(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to create a new desk")
	}
	defer cleanUp(closeCtx, action.Named(
		"remove extra desk",
		func(ctx context.Context) error {
			return removeExtraDesk(ctx, tconn)
		},
	), &retErr)
	// Wait for location-change events to be completed.
	if err := ui.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for location-change events to be completed")
	}
	// Drag the first window from overview grid to snap.
	w, err := ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the browser window in the overview mode")
	}
	if err := pc.Drag(
		w.OverviewInfo.Bounds.CenterPoint(),
		// Skip dragStartWaitTime because it's not important that the
		// drag start exactly from the center of the overview window.
		pc.DragTo(snapLeftPoint, duration),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag browser window from overview to snap")
	}
	// Wait for location-change events to be completed.
	if err := ui.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for location-change events to be completed")
	}
	// Drag divider.
	testing.ContextLog(ctx, "Dragging the divider between a snapped browser window and an overview window")
	if err := dragAndRestore(ctx, tconn, pc, duration, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, dividerDragError)
	}
	// Drag the second window to another desk to obtain an empty overview grid.
	w, err = ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the window in the overview mode to drag to another desk")
	}
	deskMiniViews, err := ui.NodesInfo(ctx, nodewith.ClassName("DeskMiniView"))
	if err != nil {
		return errors.Wrap(err, "failed to get desk mini-views")
	}
	if deskMiniViewCount := len(deskMiniViews); deskMiniViewCount < 2 {
		return errors.Errorf("expected more than 1 desk mini-views; found %v", deskMiniViewCount)
	}
	if err := pc.Drag(
		w.OverviewInfo.Bounds.CenterPoint(),
		// Skip dragStartWaitTime because it's not important that the
		// drag start exactly from the center of the overview window.
		pc.DragTo(deskMiniViews[1].Location.CenterPoint(), duration),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag browser window from overview grid to desk mini-view")
	}
	// Wait for location-change events to be completed.
	if err := ui.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for location-change events to be completed")
	}
	// Drag divider.
	testing.ContextLog(ctx, "Dragging the divider between a snapped browser window and an empty overview grid")
	if err := dragAndRestore(ctx, tconn, pc, duration, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, dividerDragError)
	}

	return nil
}
