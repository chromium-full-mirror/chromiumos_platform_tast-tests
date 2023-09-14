// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	internal "go.chromium.org/tast-tests/cros/local/chrome/internal/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var pollOptions = &testing.PollOptions{Timeout: 10 * time.Second}

// WaitForLacrosWindow waits for a Lacros window to be open and have the title to be visible if it is specified as a param.
func WaitForLacrosWindow(ctx context.Context, tconn *chrome.TestConn, title string) error {
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		if !w.IsVisible {
			return false
		}
		if !strings.HasPrefix(w.Name, "ExoShellSurface") {
			return false
		}
		if len(title) > 0 {
			return strings.HasPrefix(w.Title, title)
		}
		return true
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrapf(err, "failed to wait for lacros-chrome window to be visible (title: %v)", title)
	}
	return nil
}

// CloseLacros closes the given lacros-chrome, if it is non-nil. Otherwise, it does nothing.
func CloseLacros(ctx context.Context, l *Lacros) {
	if l != nil {
		l.Close(ctx) // Ignore error.
	}
}

// IsLacrosRunning checks if Lacros is running or not.
// NOTE: The notion of "running" here corresponds to the ChromeOS shelf indication,
// so it is false when Lacros is running in the background (without windows).
func IsLacrosRunning(ctx context.Context, cr *chrome.Chrome) (bool, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get TestConn to check Lacros")
	}
	return ash.AppRunning(ctx, tconn, apps.Lacros.ID)
}

// WaitForLacrosNotRunning waits for the Lacros is not running.
func WaitForLacrosNotRunning(ctx context.Context, tconn *chrome.TestConn, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		proc, err := lacrosproc.Root(ctx, tconn)
		if err != nil {
			if err == procutil.ErrNotFound {
				// No lacros processes are running.
				return nil
			}
			return testing.PollBreak(err)
		}

		isRunning, err := proc.IsRunning()
		if err != nil {
			return testing.PollBreak(err)
		}
		if isRunning {
			return errors.New("lacros process is still running")
		}

		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: time.Second})
}

// ResetState terminates Lacros and removes its user data directory, unless KeepAlive is enabled.
var ResetState = internal.ResetState
