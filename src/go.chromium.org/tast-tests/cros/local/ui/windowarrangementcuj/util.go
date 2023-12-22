// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package windowarrangementcuj contains helper util and test code for
// WindowArrangementCUJ.
package windowarrangementcuj

import (
	"context"
	"regexp"
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
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// dragStartWaitTime specifies how long to wait to ensure that a
	// drag is properly initiated before moving the pointer. I think
	// this is only an issue for mouse dragging, but it can be used
	// also for touch dragging, just for fair performance comparison
	// between clamshell and tablet test variants.
	dragStartWaitTime = time.Second
	// dragEndWaitTime specifies how long to wait to ensure that the
	// final position of the pointer is processed before ending a
	// drag. I think this is only an issue for mouse dragging, but it
	// can be used also for touch dragging, just for fair performance
	// comparison between clamshell and tablet test variants.
	dragEndWaitTime = time.Second
	// longPressDuration specifies how long to wait so a touch drag
	// is initiated with a long press. This is useful for dragging a
	// window from overview, or a tab from the web UI tab strip.
	longPressDuration = 2 * time.Second
)

// TestParam holds parameters of window arrangement cuj test variations.
type TestParam struct {
	BrowserType browser.Type
	Tablet      bool
}

// cleanUp is used to execute a given cleanup action and report
// the resulting error if it is not nil. The intended usage is:
//
//	func Example(ctx, closeCtx context.Context) (retErr error) {
//	  ...
//	  defer cleanUp(closeCtx, action.Named("description of cleanup action", cleanup), &retErr)
//	  ...
//	}
func cleanUp(ctx context.Context, cleanup action.Action, retErr *error) {
	if err := cleanup(ctx); err != nil {
		if *retErr == nil {
			*retErr = err
		} else {
			testing.ContextLog(ctx, "Cleanup failed: ", err)
			testing.ContextLog(ctx, "Note: This cleanup failure is not the first error. The first error will be reported after all cleanup actions have been attempted")
		}
	}
}

// combineTabs is used to merge two browser windows, each consisting
// of a single tab, into one browser window with two tabs.
func combineTabs(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context, duration time.Duration) (retErr error) {
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		return errors.Wrap(err, "failed to ensure clamshell mode")
	}
	defer func() {
		if err := cleanup(ctx); retErr == nil && err != nil {
			retErr = errors.Wrap(err, "failed to clean up after ensuring clamshell mode")
		}
	}()

	tab := nodewith.Role(role.Tab).HasClass("Tab")
	tabPIP := tab.NameContaining("/pip.html - Video playing in picture-in-picture mode")
	tabNoPIP := tab.NameRegex(regexp.MustCompile(`/pip.html - Memory usage - (\d+.\d+) MB$`))

	firstTabRect, err := ui.Location(ctx, tabNoPIP)
	if err != nil {
		return errors.Wrap(err, "failed to get the location of the first tab")
	}
	secondTabRect, err := ui.Location(ctx, tabPIP)
	if err != nil {
		return errors.Wrap(err, "failed to get the location of the second tab")
	}

	if err := pc.Drag(
		firstTabRect.CenterPoint(),
		uiauto.Sleep(dragStartWaitTime),
		pc.DragTo(firstTabRect.RightCenter(), duration),
		pc.DragTo(secondTabRect.CenterPoint(), duration),
		uiauto.Sleep(dragEndWaitTime),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag one browser tab to the other")
	}

	ws, err := getAllNonPipWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the window list")
	}
	if len(ws) != 1 {
		return errors.Errorf("unexpected number of windows after trying to merge: got %d; expected 1", len(ws))
	}

	return nil
}

// removeExtraDesk removes the active desk and then ensures that a window that
// was on this removed desk is active. This window activation is important for
// the drag in combineTabs, because the tab to be dragged needs to be on top.
func removeExtraDesk(ctx context.Context, tconn *chrome.TestConn) error {
	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool { return w.OnActiveDesk })
	if err != nil {
		return errors.Wrap(err, "failed to find window on active desk")
	}
	if err := ash.RemoveActiveDesk(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to remove desk")
	}
	if err := w.ActivateWindow(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to ensure suitable window activation")
	}
	return nil
}

// dragAndRestore performs a drag beginning at the first given point, proceeding
// through the others in order, and ending back at the first given point. Before
// ending the drag, dragAndRestore waits until every window has the same bounds
// as before the drag (as expected because the drag is a closed loop).
func dragAndRestore(ctx context.Context, tconn *chrome.TestConn, pc pointer.Context, duration time.Duration, p ...coords.Point) error {
	if len(p) < 2 {
		return errors.Errorf("expected at least two drag points, got %v", p)
	}

	wsInitial, err := getAllNonPipWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get windows")
	}

	verifyBounds := func(ctx context.Context) error {
		for _, wInitial := range wsInitial {
			wNow, err := ash.GetWindow(ctx, tconn, wInitial.ID)
			if err != nil {
				return errors.Wrapf(err, "failed to look up %q window by ID %d (the app probably crashed)", wInitial.Title, wInitial.ID)
			}
			// Window widths may be adjusted by up to this amount when in split screen mode.
			// The right and left side can vary up to half the divider thickness.
			const splitScreenDividerHalfThickness int = 4
			if !coords.CompareBoundsWithMargins(wInitial.BoundsInRoot, wNow.BoundsInRoot, splitScreenDividerHalfThickness, 0, splitScreenDividerHalfThickness, 0) {
				return errors.Errorf("%q window bounds not restored; changed from %v to %v", wNow.Title, wInitial.BoundsInRoot, wNow.BoundsInRoot)
			}
		}
		return nil
	}
	verifyBoundsTimeout := &testing.PollOptions{Timeout: 2 * time.Minute}

	dragSteps := []uiauto.Action{uiauto.Sleep(dragStartWaitTime)}
	for i := 1; i < len(p); i++ {
		dragSteps = append(dragSteps, pc.DragTo(p[i], duration))
	}
	dragSteps = append(dragSteps, pc.DragTo(p[0], duration), func(ctx context.Context) error {
		if err := testing.Poll(ctx, verifyBounds, verifyBoundsTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for expected window bounds before ending drag")
		}
		return nil
	})
	if err := pc.Drag(p[0], dragSteps...)(ctx); err != nil {
		return errors.Wrap(err, "failed to drag")
	}

	return nil
}

// getSplitViewDragPoints computes points in DIPs, useful for drags related to
// split view. The points are chosen so that if you convert the coordinates to
// input.TouchCoord, they will remain inside the display bounds, despite the
// rounding error in that conversion.
func getSplitViewDragPoints(info *display.Info) (
	splitViewDragPoints []coords.Point,
	snapLeftPoint, snapRightPoint coords.Point,
) {
	// The calculation of right can be understood by imagining the work area as
	// a grid of pixels with info.WorkArea.Right() columns numbered from 0 to
	// info.WorkArea.Right() - 1. The rightmost x-coordinate is
	// info.WorkArea.Right() - 1, and we can use info.WorkArea.Right() - 2 to
	// allow for rounding error in case of conversion to input.TouchCoord.
	right := info.WorkArea.Right() - 2
	// All points computed by getSplitViewDragPoints are vertically centered.
	y := info.WorkArea.CenterY()

	// splitViewDragPoints gives the trajectory for a split view resizing drag.
	splitViewDragPoints = []coords.Point{
		// Start from the center, assuming that the split view divider is there.
		info.WorkArea.CenterPoint(),
		// Drag to one-quarter position, so that the left snapped window (if
		// any) will probably reach its minimum size.
		coords.NewPoint(info.WorkArea.Left+info.WorkArea.Width/4, y),
		// Drag all the way to the right.
		coords.NewPoint(right, y),
		// Dragging back to the first point is implied. See dragAndRestore.
	}
	// snapLeftPoint is a point where a window drag can end to snap the dragged
	// window on the left. info.WorkArea.Left is the leftmost x-coordinate in
	// the work area, and we can use info.WorkArea.Left + 1 to allow for
	// rounding error in case of conversion to input.TouchCoord.
	snapLeftPoint = coords.NewPoint(info.WorkArea.Left+1, y)
	// snapRightPoint is a point where a window drag can end to snap the
	// dragged window on the right.
	snapRightPoint = coords.NewPoint(right, y)

	return
}

// getAllNonPipWindows calls ash.GetAllWindows and filters out PIP windows
// because they are not supposed to be returned by ash.GetAllWindows in the
// first place (see b/252552657#comment7).
// TODO(b/252552657): When the bug is fixed, remove this and update callers to
// use ash.GetAllWindows directly.
func getAllNonPipWindows(ctx context.Context, tconn *chrome.TestConn) ([]*ash.Window, error) {
	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return nil, err
	}

	var filtered []*ash.Window
	for _, w := range ws {
		if w.State != ash.WindowStatePIP {
			filtered = append(filtered, w)
		}
	}
	return filtered, nil
}

func setOverviewModeAndWait(ctx context.Context, tconn *chrome.TestConn) error {
	kw, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kw.Close(ctx)

	topRow, err := input.KeyboardTopRowLayout(ctx, kw)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the top-row layout")
	}

	waitForOverviewState := func(ctx context.Context) error {
		return ash.WaitForOverviewState(ctx, tconn, ash.Shown, 30*time.Second)
	}

	ui := uiauto.New(tconn)
	return uiauto.NamedCombine("set overview mode and wait",
		// Add a stabilization delay before entering overview mode to
		// mitigate the potential timing issue between desktop
		// splitting and entering overview mode.
		ui.WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged),
		kw.AccelAction(topRow.SelectTask),
		waitForOverviewState,
	)(ctx)
}
