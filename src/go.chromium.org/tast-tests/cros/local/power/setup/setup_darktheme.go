// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This part follows the name shown on ChromeOS UI: Dark Theme.
// It is often mentioned as "Dark Mode".

package setup

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/errors"
)

// TurnOnDarkTheme turns dark theme on.
// Reset to auto theme (OS default after logging in) when cleaning power test setup.
func TurnOnDarkTheme(ctx context.Context, c *chrome.TestConn) (CleanupCallback, error) {
	if err := setAutoThemeMode(ctx, c, true /* enabled */); err != nil {
		return nil, errors.Wrap(err, "failed to enable dark theme")
	}
	// TODO(b/267686390): Add CleanupCallback when more controls over dark theme scheduler
	// are supported.
	return nil, nil
}

// TurnOnLightTheme turns light theme on.
// Reset to auto theme (OS default after logging in) when cleaning power test setup.
func TurnOnLightTheme(ctx context.Context, c *chrome.TestConn) (CleanupCallback, error) {
	if err := setAutoThemeMode(ctx, c, false /* enabled */); err != nil {
		return nil, errors.Wrap(err, "failed to enable light theme")
	}
	// TODO(b/267686390): Add CleanupCallback when more controls over dark theme scheduler
	// are supported.
	return nil, nil
}

// setAutoThemeMode sets the theme to dark or light theme through autotest API.
func setAutoThemeMode(ctx context.Context, c *chrome.TestConn, enabled bool) error {
	return c.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.forceAutoThemeMode)`, enabled)
}
