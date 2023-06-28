// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputsimulations

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
)

// DoAshWorkflows performs a series of system UI interactions
// to generate samples of Ash.Smoothness.PercentDroppedFrames_1sWindow
// (ADF). This function is self contained, in that at the end of the
// function, the state of the device is the same as when DoAshWorkflows
// was first called, except for the location of the mouse. This function
// assumes that when it is called, the device is not in overview mode.
// This function also assumes that at least 1 window is opened at the
// time of the function call. Lastly, this function assumes that the
// system tray is visible.
//
// The action consists of opening and closing the system tray, entering
// overview mode, dragging a window preview around, and exiting
// overview mode.
func DoAshWorkflows(ctx context.Context, tconn *chrome.TestConn, pc pointer.Context) error {
	if err := DoOverviewWorkflow(ctx, tconn, pc); err != nil {
		return errors.Wrap(err, "failed to interact with a window in overview mode")
	}

	if err := DoSystemTrayWorkflow(ctx, tconn, pc); err != nil {
		return errors.Wrap(err, "failed to interact with system tray")
	}
	return nil
}

// DoSystemTrayWorkflow opens and closes the system tray.
func DoSystemTrayWorkflow(ctx context.Context, tconn *chrome.TestConn, pc pointer.Context) error {
	touch, err := isTablet(pc)
	if err != nil {
		return err
	}

	cleanup, err := quicksettings.Init(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to init quicksettings")
	}
	defer cleanup()

	// Open and close the system tray bubble.
	// There is more than one "UnifiedSystemTray" on the UI tree when an extended display is connected.
	systemTray := quicksettings.SystemTray.First()
	quickSettingsRoot := quicksettings.GetRootFinder()
	ac := uiauto.New(tconn)

	return uiauto.Combine(
		"open and close the status tray",
		func(ctx context.Context) error {
			if !touch {
				return ac.MouseMoveTo(systemTray, 500*time.Millisecond)(ctx)
			}
			return nil
		},
		pc.Click(systemTray),
		ac.WaitUntilExists(quickSettingsRoot),
		// Add a fixed sleep to simulate a user looking for the button that
		// they want to press.
		uiauto.Sleep(500*time.Millisecond),
		pc.Click(systemTray),
		ac.WaitUntilGone(quickSettingsRoot),
	)(ctx)
}

// DoOverviewWorkflow enters overview mode, drags the first window
// to the left and to the right, and exits overview mode. This function
// assumes at least 1 window is opened at the time of the function call.
func DoOverviewWorkflow(ctx context.Context, tconn *chrome.TestConn, pc pointer.Context) error {
	touch, err := isTablet(pc)
	if err != nil {
		return err
	}

	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		return errors.Wrap(err, "failed to enter overview mode")
	}

	w, err := ash.FindFirstWindowInOverview(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get overview window")
	}

	dragPoint := w.OverviewInfo.Bounds.CenterPoint()
	if !touch {
		if err := mouse.Move(tconn, dragPoint, time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to move mouse to the first window in overview mode")
		}
	}

	// Long press the window preview in tablet mode to properly drag
	// the preview.
	var longPressDuration time.Duration
	if touch {
		longPressDuration = time.Second
	}

	offset := coords.NewPoint(50, 0)
	if err := uiauto.Combine(
		"drag window to the right and left",
		pc.Drag(
			dragPoint,
			uiauto.Sleep(longPressDuration),
			pc.DragTo(dragPoint.Add(offset), 500*time.Millisecond),
			pc.DragTo(dragPoint.Sub(offset), time.Second),
			pc.DragTo(dragPoint, 500*time.Millisecond),
		),

		// Sleep to give a fixed amount of time for the preview to
		// stabilize. A fixed value helps keep the overall duration of
		// DoOverviewWorkflow relatively consistent.
		uiauto.Sleep(time.Second),
	)(ctx); err != nil {
		return err
	}

	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		return errors.Wrap(err, "failed to exit overview mode")
	}
	return nil
}

func isTablet(pc pointer.Context) (bool, error) {
	switch pc.(type) {
	case nil:
		return false, errors.New("pointer context is nil")
	case *pointer.MouseContext:
		return false, nil
	case *pointer.TouchContext:
		return true, nil
	default:
		return false, errors.New("unrecognized pointer context type")
	}

}
