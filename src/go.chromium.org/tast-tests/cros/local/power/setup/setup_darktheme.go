// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This part follows the name shown on ChromeOS UI: Dark Theme.
// It is often mentioned as "Dark Mode".

package setup

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/errors"
)

// TurnOnDarkTheme turns dark theme on.
// Reset to auto theme (OS default after logging in) when cleaning power test setup.
func TurnOnDarkTheme(ctx context.Context, c *chrome.TestConn) (CleanupCallback, error) {
	if err := setTheme(ctx, c, personalization.DarkModeName); err != nil {
		return nil, errors.Wrap(err, "failed to enable dark theme")
	}
	// TODO(b/267686390): Add CleanupCallback when more controls over dark theme scheduler
	// are supported.
	return nil, nil
}

// TurnOnLightTheme turns light theme on.
// Reset to auto theme (OS default after logging in) when cleaning power test setup.
func TurnOnLightTheme(ctx context.Context, c *chrome.TestConn) (CleanupCallback, error) {
	if err := setTheme(ctx, c, personalization.LightModeName); err != nil {
		return nil, errors.Wrap(err, "failed to enable light theme")
	}
	// TODO(b/267686390): Add CleanupCallback when more controls over dark theme scheduler
	// are supported.
	return nil, nil
}

// setTheme opens the "Wallpaper & style" app from settings and toggles the theme as |theme|.
func setTheme(ctx context.Context, c *chrome.TestConn, themeName string) error {
	ui := uiauto.New(c).WithTimeout(30 * time.Second)

	var toggleThemeAction uiauto.Action
	switch themeName {
	case personalization.AutoModeName:
		toggleThemeAction = personalization.ToggleAutoMode(ui)
	case personalization.DarkModeName:
		toggleThemeAction = personalization.ToggleDarkMode(ui)
	case personalization.LightModeName:
		toggleThemeAction = personalization.ToggleLightMode(ui)
	default:
		return errors.Errorf("unknown theme: %s", themeName)
	}

	return uiauto.Combine(fmt.Sprintf("enable %s theme", themeName),
		personalization.OpenPersonalizationHub(ui),
		toggleThemeAction,
		personalization.ClosePersonalizationHub(ui),
	)(ctx)
}
