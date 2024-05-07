// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memorystress

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// reloadCrashedTab reloads the active tab if it's crashed. Returns whether the tab is reloaded.
func reloadCrashedTab(ctx context.Context, br *browser.Browser) (bool, error) {
	tabURL, err := activeTabURL(ctx, br)
	if err != nil {
		return false, errors.Wrap(err, "cannot get active tab URL")
	}

	// If the active tab's URL is not in the devtools targets, the active tab is crashed.
	targetAvailable, err := isTargetAvailable(ctx, br, chrome.MatchTargetURL(tabURL))
	if err != nil {
		return false, errors.Wrap(err, "isTargetAvailable failed")
	}

	if !targetAvailable {
		testing.ContextLog(ctx, "Reload tab:", tabURL)
		if err := br.ReloadActiveTab(ctx); err != nil {
			return false, errors.Wrap(err, "failed to reload active tab")
		}
		if err := waitAllocationForURL(ctx, br, tabURL); err != nil {
			return false, errors.Wrap(err, "waitAllocationForURL failed")
		}
		return true, nil
	}
	return false, nil
}

// waitMoveCursor moves the mouse cursor until after the specified waiting time. Moving the cursor provides a visual hint of jankiness.
func waitMoveCursor(ctx context.Context, mw *input.MouseEventWriter, d time.Duration) error {
	// Reset the cursor to the top left.
	mw.Move(-10000, -10000)

	const sleepTime = 15 * time.Millisecond
	total := time.Duration(0)
	i := 0
	for total < d {
		// Moves mouse cursor back and forth diagonally.
		if i%100 < 50 {
			mw.Move(5, 5)
		} else {
			mw.Move(-5, -5)
		}
		// GoBigSleepLint: Sleeps briefly after each cursor move.
		if err := testing.Sleep(ctx, sleepTime); err != nil {
			return errors.Wrap(err, "sleep timeout")
		}
		i++
		total += sleepTime
	}

	return nil
}

// SwitchTabsList sends keyboard messages to switch tabs. It reloads crashed tabs. Returns the reload count.
//
// Switch tabs for len(waitTimeList) times. After switching, sleep for the time in waitTimeList.
func SwitchTabsList(ctx context.Context, br *browser.Browser, waitTimeList []time.Duration) (uint64, error) {
	mouse, err := input.Mouse(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "cannot initialize mouse")
	}
	defer mouse.Close(ctx)

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "cannot initialize keyboard")
	}
	defer keyboard.Close(ctx)

	var reloadCount uint64
	for i, waitTime := range waitTimeList {
		if err := keyboard.Accel(ctx, "ctrl+tab"); err != nil {
			return 0, errors.Wrap(err, "Accel(Ctrl+Tab) failed")
		}

		// Waits between tab switches.
		if err := waitMoveCursor(ctx, mouse, waitTime); err != nil {
			return 0, errors.Wrap(err, "error when moving mouse cursor")
		}
		testing.ContextLogf(ctx, "%3d, wait time: %v", i, waitTime)

		reloaded, err := reloadCrashedTab(ctx, br)
		if err != nil {
			return 0, errors.Wrap(err, "reloadCrashedTab failed")
		}
		if reloaded {
			reloadCount++
		}
	}
	return reloadCount, nil
}
