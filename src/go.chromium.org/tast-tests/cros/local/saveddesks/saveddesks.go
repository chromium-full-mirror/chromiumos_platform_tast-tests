// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package saveddesks contains utility functions for saveddesk tests.
package saveddesks

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VerifyWindowCount verifies that there are `windowCount` app windows.
func VerifyWindowCount(ctx context.Context, tconn *chrome.TestConn, windowCount int) error {
	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get all open windows")
	}

	if len(ws) != windowCount {
		// TODO(b/241118477): Remove exiting out of the location access popup step
		// once there is a more permanent fix to disable the popup. The popup is not
		// captured by the UI tree, so it is unable to interact with it using uiauto
		// libraries. In order to remove this popup window,
		// we will need to input keyboard commands.
		if len(ws) == windowCount+1 {
			// We need to press the enter key twice.
			// Once to get get the focus onto the popup window
			// and once to confirm a choice to exit the window.
			kb, err := input.Keyboard(ctx)
			if err != nil {
				return errors.Wrap(err, "cannot create keyboard")
			}
			defer kb.Close(ctx)
			if err := kb.Accel(ctx, "Enter"); err != nil {
				return errors.Wrap(err, "cannot press 'Enter'")
			}
			if err := kb.Accel(ctx, "Enter"); err != nil {
				return errors.Wrap(err, "cannot press 'Enter'")
			}
		}
		// Verify that there is now the correct number of windows.
		// We use testing.Poll because window closures are asynchronous.
		// The dismissed popup or other closing windows may take a brief moment
		// to fully disappear from Ash's window hierarchy.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			wc, err := ash.GetAllWindows(ctx, tconn)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get all open windows"))
			}
			if len(wc) != windowCount {
				return errors.Errorf("found inconsistent number of window(s): got %v, want %v", len(wc), windowCount)
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
			return err
		}
	}

	return nil
}

// WaitforAppsToLaunch waits for the given apps to launch.
func WaitforAppsToLaunch(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, appsList []apps.App) error {
	for _, app := range appsList {
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			return errors.Wrapf(err, "%s did not appear in shelf after launch", app.Name)
		}
		// Some apps may take a long time to load such as Play Store. Wait for launch event to be completed.
		if _, err := ash.WaitForAppWindow(ctx, tconn, app.ID); err != nil {
			return errors.Wrapf(err, "failed to wait for the %s app launch event to be completed", app.Name)
		}
	}

	if err := VerifyWindowCount(ctx, tconn, len(appsList)); err != nil {
		return errors.Wrap(err, "failed to verify window count")
	}

	return nil
}

// WaitforAppsToBeVisible waits for the windows of the given apps to be visible.
func WaitforAppsToBeVisible(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, appsList []apps.App) error {
	for _, app := range appsList {
		// Wait for the launched app window to become visible.
		if _, err := ash.WaitForAppWindow(ctx, tconn, app.ID); err != nil {
			return errors.Wrapf(err, "failed to wait for the %s app launch event to finish", app.Name)
		}
		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return w.IsVisible
		}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
			return errors.Wrapf(err, "%s app window not visible after launching", app.Name)
		}
	}

	return nil
}

// OpenApps opens the given apps, waits for them to launch and their windows to appear.
func OpenApps(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context, appsList []apps.App) error {
	for _, app := range appsList {
		if err := apps.Launch(ctx, tconn, app.ID); err != nil {
			return errors.Wrapf(err, "failed to open %s", app.Name)
		}
	}

	if err := WaitforAppsToLaunch(ctx, tconn, ac, appsList); err != nil {
		return errors.Wrap(err, "failed to wait for app launch")
	}

	return nil
}

// DeleteSavedDesks resets the  current state of the system by deleting
// all newly created saved desks.
func DeleteSavedDesks(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context) error {
	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		return errors.Wrap(err, "failed to set overview mode")
	}
	if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the animation to be completed")
	}
	// Check if library button is visible.
	libraryButtonVisible, err := ash.IsLibraryButtonVisible(ctx, ac)
	if err != nil {
		return errors.Wrap(err, "failed to check if library is visible")
	}

	// Enter library page, and delete all saved desks.
	if libraryButtonVisible {
		if err := ash.EnterLibraryPage(ctx, ac); err != nil {
			return errors.Wrap(err, "failed to enter library page")
		}
		if err := ash.DeleteAllSavedDesks(ctx, ac, tconn); err != nil {
			return errors.Wrap(err, "fail to clean up desk templates")
		}
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		return errors.Wrap(err, "failed to set overview mode")
	}
	if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the animation to be completed")
	}
	return nil
}
