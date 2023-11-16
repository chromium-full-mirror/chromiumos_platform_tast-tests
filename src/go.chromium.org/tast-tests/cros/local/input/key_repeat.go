// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

const (
	// Chrome pref names
	keyRepeatEnabledPrefName  = "settings.language.xkb_auto_repeat_enabled_r2"
	keyRepeatDelayPrefName    = "settings.language.xkb_auto_repeat_delay_r2"
	keyRepeatIntervalPrefName = "settings.language.xkb_auto_repeat_interval_r2"
)

// KeyRepeatSettings represents a settings of ChromeOS key repeat.
type KeyRepeatSettings struct {
	Enabled  bool
	Delay    int
	Interval int
}

// CurrentKeyRepeatSettings returns the current key repeat settings of the device.
func CurrentKeyRepeatSettings(ctx context.Context, tconn *chrome.TestConn) (KeyRepeatSettings, error) {
	var enabled struct {
		Value bool `json:"value"`
	}
	if err := tconn.Call(ctx, &enabled, "tast.promisify(chrome.settingsPrivate.getPref)", keyRepeatEnabledPrefName); err != nil {
		return KeyRepeatSettings{}, errors.Wrap(err, "failed to get keyRepeatEnabled")
	}
	var delay struct {
		Value int `json:"value"`
	}
	if err := tconn.Call(ctx, &delay, "tast.promisify(chrome.settingsPrivate.getPref)", keyRepeatDelayPrefName); err != nil {
		return KeyRepeatSettings{}, errors.Wrap(err, "failed to get keyRepeatDelay")
	}
	var interval struct {
		Value int `json:"value"`
	}
	if err := tconn.Call(ctx, &interval, "tast.promisify(chrome.settingsPrivate.getPref)", keyRepeatIntervalPrefName); err != nil {
		return KeyRepeatSettings{}, errors.Wrap(err, "failed to get keyRepeatInterval")
	}

	return KeyRepeatSettings{enabled.Value, delay.Value, interval.Value}, nil
}

// SetKeyRepeatSettings sets the key repeat settings of the device.
func SetKeyRepeatSettings(ctx context.Context, tconn *chrome.TestConn, settings KeyRepeatSettings) error {
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", keyRepeatEnabledPrefName, settings.Enabled); err != nil {
		return errors.Wrap(err, "failed to set keyRepeatEnabled")
	}
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", keyRepeatDelayPrefName, settings.Delay); err != nil {
		return errors.Wrap(err, "failed to set keyRepeatDelay")
	}
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", keyRepeatIntervalPrefName, settings.Interval); err != nil {
		return errors.Wrap(err, "failed to set keyRepeatInterval")
	}
	return nil
}

// EnsureKeyRepeatEnabled makes sure that the key repeat settings are enabled or disabled,
// and returns a function which reverts back to the original settings.
func EnsureKeyRepeatEnabled(ctx context.Context, tconn *chrome.TestConn, enabled bool) (func(ctx context.Context, tconn *chrome.TestConn) error, error) {
	originalSettings, err := CurrentKeyRepeatSettings(ctx, tconn)
	if err != nil {
		return nil, err
	}
	if originalSettings.Enabled != enabled {
		s := KeyRepeatSettings{enabled, originalSettings.Delay, originalSettings.Interval}
		if err := SetKeyRepeatSettings(ctx, tconn, s); err != nil {
			return nil, err
		}
	}

	// Always revert to the original settings so it can always be back to the original settings
	// even when the settings changes during the test.
	return func(ctx context.Context, tconn *chrome.TestConn) error {
		return SetKeyRepeatSettings(ctx, tconn, originalSettings)
	}, nil
}
