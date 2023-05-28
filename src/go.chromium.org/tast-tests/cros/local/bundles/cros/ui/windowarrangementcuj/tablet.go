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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// exerciseSplitViewResize assumes two snapped windows and a second desk (but the
// first desk is the active one). Then exerciseSplitViewResize does the following:
// 1. Drag the divider.
// 2. Enter overview.
// 3. Drag the divider.
// 4. Drag the overview window to the second desk.
// 5. Drag the divider.
func exerciseSplitViewResize(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context, enterOverview action.Action, splitViewDragPoints ...coords.Point) error {
	const (
		slow         = 2 * time.Second
		moderatePace = time.Second
	)

	// 1. Drag the divider.
	testing.ContextLog(ctx, "Dragging the divider between two snapped windows")
	if err := dragAndRestore(ctx, tconn, pc, slow, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, "failed to drag divider between two snapped windows")
	}

	// 2. Enter overview.
	if err := enterOverview(ctx); err != nil {
		return errors.Wrap(err, "failed to enter overview mode")
	}

	// 3. Drag the divider.
	testing.ContextLog(ctx, "Dragging the divider between an overview window and a snapped window")
	if err := dragAndRestore(ctx, tconn, pc, slow, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, "failed to drag divider between overview window and snapped window")
	}

	// 4. Drag the overview window to the second desk.
	w, err := ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find the window in the overview mode to drag to the second desk")
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
		uiauto.Sleep(longPressDuration),
		pc.DragTo(deskMiniViews[1].Location.CenterPoint(), moderatePace),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag overview window to second desk")
	}
	// Wait for the overview window to reach the second desk before dragging the divider. This is
	// important because the state at the beginning of the divider drag determines which performance
	// metrics are recorded for the entire drag. If the drag begins with a window still in overview,
	// the resulting data will be for Ash.SplitViewResize.PresentationTime.TabletMode.WithOverview
	// and not Ash.SplitViewResize.PresentationTime.TabletMode.SingleWindow.
	if err := ui.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for location-change events to be completed")
	}

	// 5. Drag the divider.
	testing.ContextLog(ctx, "Dragging the divider between an empty overview grid and a snapped window")
	if err := dragAndRestore(ctx, tconn, pc, slow, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, "failed to drag divider between empty overview grid and snapped window")
	}

	return nil
}

// RunTablet runs window arrangement cuj for tablet. Since windows are always
// maximized in tablet mode, we only test performance for tab dragging and split
// view resizing.
func RunTablet(ctx, closeCtx context.Context, br *browser.Browser, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context) (retErr error) {
	const (
		timeout           = 10 * time.Second
		duration          = 2 * time.Second
		doubleTapInterval = 100 * time.Millisecond
	)

	// Gets primary display info and interesting drag points.
	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the primary display info")
	}
	splitViewDragPoints, _, snapRightPoint := getSplitViewDragPoints(info)

	tabStripButton := nodewith.Role(role.Button).ClassName("WebUITabCounterButton").First()
	if err := pc.Click(tabStripButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the tab strip button")
	}

	if err := ui.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the tab strip to stabilize")
	}

	tabStripConn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL("chrome://tab-strip.top-chrome/"))
	if err != nil {
		return errors.Wrap(err, "failed to get connection to web UI tab strip")
	}
	defer cleanUp(closeCtx, action.Named(
		"close connection to web UI tab strip",
		func(ctx context.Context) error {
			return tabStripConn.Close()
		},
	), &retErr)

	// Get the bounds of the first tab by using JavaScript directly,
	// because uiauto can't get the accurate bounds reliably in Lacros.
	var firstTabRect coords.Rect
	if err := tabStripConn.Eval(ctx,
		`(function() {
			const b = document.querySelector('tabstrip-tab-list').shadowRoot
				.querySelectorAll('tabstrip-tab')[0].shadowRoot
				.getElementById('tab').getBoundingClientRect();
			return {
				'left':   Math.round(b.left),
				'top':    Math.round(b.top),
				'width':  Math.round(b.width),
				'height': Math.round(b.height),
			};
		})()`,
		&firstTabRect); err != nil {
		return errors.Wrap(err, "failed to get bounds of first tab")
	}

	// firstTabRect is relative to the web UI tab strip which is across the top of
	// the browser window, so we can treat firstTabRect as relative to the browser
	// window. Then here we convert it to be relative to the root window instead.
	ws, err := getAllNonPipWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get windows")
	}
	if len(ws) != 1 {
		return errors.Errorf("unexpected number of windows: got %d; want 1", len(ws))
	}
	browserRect := ws[0].TargetBounds
	firstTabRect = firstTabRect.WithOffset(browserRect.Left, browserRect.Top)

	// Drag the first tab in the tab strip and snap it to the right.
	defer cleanUp(ctx, action.Named(
		"maximize the window",
		func(ctx context.Context) error {
			ws, err := getAllNonPipWindows(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to get windows")
			}
			if len(ws) != 1 {
				return errors.Errorf("unexpected number of windows: got %d; want 1", len(ws))
			}
			if err := ash.SetWindowStateAndWait(ctx, tconn, ws[0].ID, ash.WindowStateMaximized); err != nil {
				return errors.Wrap(err, "failed to set browser window state to \"Maximized\"")
			}
			return nil
		},
	), &retErr)
	if err := pc.Drag(
		firstTabRect.CenterPoint(),
		uiauto.Sleep(longPressDuration),
		pc.DragTo(snapRightPoint, 3*time.Second),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag a tab to snap to the right")
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

	setOverviewMode := func(ctx context.Context) error {
		return setOverviewModeAndWait(ctx, tconn)
	}

	// Exercise split view resize functionality.
	if err := exerciseSplitViewResize(ctx, tconn, ui, pc, setOverviewMode, splitViewDragPoints...); err != nil {
		return errors.Wrap(err, "failed to exercise split view resize functionality with two browser windows")
	}

	return nil
}
