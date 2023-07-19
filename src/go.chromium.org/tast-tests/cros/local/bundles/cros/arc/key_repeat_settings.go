// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Chrome pref names
	keyRepeatEnabledPrefName  = "settings.language.xkb_auto_repeat_enabled_r2"
	keyRepeatDelayPrefName    = "settings.language.xkb_auto_repeat_delay_r2"
	keyRepeatIntervalPrefName = "settings.language.xkb_auto_repeat_interval_r2"
	// ARC settings names
	keyRepeatDelaySettingsName    = "key_repeat_timeout"
	keyRepeatIntervalSettingsName = "key_repeat_delay"
)

type keyRepeatSettings struct {
	enabled  bool
	delay    int
	interval int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeyRepeatSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks ChromeOS key repeat settings are applied to ARC++ settings",
		Contacts:     []string{"arc-framework+tast@google.com", "nergi@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "android_vm_t"},
		Fixture:      "arcBooted",
		Timeout:      3 * time.Minute,
	})
}

func KeyRepeatSettings(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	p := s.FixtValue().(*arc.PreData)
	cr := p.Chrome
	a := p.ARC
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	// Defer cleanup
	currentSettings, err := chromeKeyRepeatSettings(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get current keyRepeatSettings: ", err)
	}
	defer func(ctx context.Context) {
		if err := setKeyRepeatSettings(ctx, tconn, currentSettings); err != nil {
			s.Fatal("Failed to set keyRepeatSettings: ", err)
		}
	}(cleanupCtx)

	// Set expected settings values.
	expected := keyRepeatSettings{true, 123, 1000}
	if err := setKeyRepeatSettings(ctx, tconn, expected); err != nil {
		s.Fatal("Failed to set keyRepeatSettings: ", err)
	}

	// Validate settings.
	if err := validateKeyRepeatSettings(ctx, a, expected); err != nil {
		s.Fatal("Failed to validate keyRepeatSettings: ", err)
	}
}

func arcSecureSettingsValue(ctx context.Context, a *arc.ARC, name string) (int, error) {
	output, err := a.Command(ctx, "settings", "get", "secure", name).Output()
	if err != nil {
		return -1, errors.Wrapf(err, "failed to run \"settings get secure %s\"", name)
	}
	value, err := strconv.Atoi(strings.Trim(string(output), "\n"))
	if err != nil {
		return -1, errors.Wrapf(err, "failed to convert %s to int", output)
	}
	return value, nil
}

func validateKeyRepeatSettings(ctx context.Context, a *arc.ARC, expected keyRepeatSettings) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		delay, err := arcSecureSettingsValue(ctx, a, keyRepeatDelaySettingsName)
		if delay != expected.delay {
			return errors.Wrapf(err, "key repeat delay %d doesn't match expected %d", delay, expected.delay)
		}

		interval, err := arcSecureSettingsValue(ctx, a, keyRepeatIntervalSettingsName)
		if interval != expected.interval {
			return errors.Wrapf(err, "key repeat interval %d doesn't match expected %d", interval, expected.interval)
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}

func chromeKeyRepeatSettings(ctx context.Context, tconn *chrome.TestConn) (keyRepeatSettings, error) {
	var enabled struct {
		Value bool `json:"value"`
	}
	if err := tconn.Call(ctx, &enabled, "tast.promisify(chrome.settingsPrivate.getPref)", keyRepeatEnabledPrefName); err != nil {
		return keyRepeatSettings{}, errors.Wrap(err, "failed to get keyRepeatEnabled")
	}
	var delay struct {
		Value int `json:"value"`
	}
	if err := tconn.Call(ctx, &delay, "tast.promisify(chrome.settingsPrivate.getPref)", keyRepeatDelayPrefName); err != nil {
		return keyRepeatSettings{}, errors.Wrap(err, "failed to get keyRepeatDelay")
	}
	var interval struct {
		Value int `json:"value"`
	}
	if err := tconn.Call(ctx, &interval, "tast.promisify(chrome.settingsPrivate.getPref)", keyRepeatIntervalPrefName); err != nil {
		return keyRepeatSettings{}, errors.Wrap(err, "failed to get keyRepeatInterval")
	}

	return keyRepeatSettings{enabled.Value, delay.Value, interval.Value}, nil
}

func setKeyRepeatSettings(ctx context.Context, tconn *chrome.TestConn, settings keyRepeatSettings) error {
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", keyRepeatEnabledPrefName, settings.enabled); err != nil {
		return errors.Wrap(err, "failed to set keyRepeatEnabled")
	}
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", keyRepeatDelayPrefName, settings.delay); err != nil {
		return errors.Wrap(err, "failed to set keyRepeatDelay")
	}
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", keyRepeatIntervalPrefName, settings.interval); err != nil {
		return errors.Wrap(err, "failed to set keyRepeatInterval")
	}
	return nil
}
