// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VerifyReturnToApp minimises the current VC app window and restores it from vcPanel.
func VerifyReturnToApp(ctx context.Context, tconn *chrome.TestConn) error {
	appWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get active window")
	}

	// Save current window state and use it for return verification.
	appWindowState := appWindow.State

	if err := ash.SetWindowStateAndWait(ctx, tconn, appWindow.ID, ash.WindowStateMinimized); err != nil {
		return errors.Wrap(err, "failed to minimize window")
	}

	vcTray := vctray.New(ctx, tconn)

	if err := uiauto.Combine("return to app via mcpanel",
		vcTray.ExpandPanel,
		// Remove the "Chrome" prefix to match the window.
		// vcTray only displays name `Meet - ~new`,
		// while in Tast API the window title is `Chrome - Meet - ~new`.
		vcTray.ReturnToApp(strings.TrimPrefix(appWindow.Title, "Chrome - ")),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to return to app")
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		newActiveWindow, err := ash.GetActiveWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get active window")
		}

		if newActiveWindow.ID != appWindow.ID || newActiveWindow.State != appWindowState {
			return errors.Errorf("failed to restore window(expected: %+v, actual: %+v)", appWindow, newActiveWindow)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}
