// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This part follows the name shown on ChromeOS UI: Dark Theme.
// It is often mentioned as "Dark Mode".

package setup

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/personalization"
	"go.chromium.org/tast/core/errors"
)

// TurnOnDarkTheme turns dark theme on.
// Reset to auto theme (OS default after logging in) when cleaning power test setup.
func TurnOnDarkTheme(ctx context.Context, c *chrome.TestConn) (CleanupCallback, error) {
	ui := uiauto.New(c).WithTimeout(30 * time.Second)

	if err := uiauto.Combine("Enable dark mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleDarkMode(ui),
		personalization.ClosePersonalizationHub(ui),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enable dark theme")
	}
	// TODO(b/267686390): Add CleanupCallback when more controls over dark theme scheduler
	// are supported.
	return nil, nil
}

// TurnOnLightTheme turns light theme on.
// Reset to auto theme (OS default after logging in) when cleaning power test setup.
func TurnOnLightTheme(ctx context.Context, c *chrome.TestConn) (CleanupCallback, error) {
	ui := uiauto.New(c).WithTimeout(30 * time.Second)

	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui),
		personalization.ClosePersonalizationHub(ui),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enable light theme")
	}
	// TODO(b/267686390): Add CleanupCallback when more controls over dark theme scheduler
	// are supported.
	return nil, nil
}
