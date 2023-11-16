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
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ARC settings names
	keyRepeatDelaySettingsName    = "key_repeat_timeout"
	keyRepeatIntervalSettingsName = "key_repeat_delay"
)

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
	currentSettings, err := input.CurrentKeyRepeatSettings(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get current keyRepeatSettings: ", err)
	}
	defer func(ctx context.Context) {
		if err := input.SetKeyRepeatSettings(ctx, tconn, currentSettings); err != nil {
			s.Fatal("Failed to set keyRepeatSettings: ", err)
		}
	}(cleanupCtx)

	// Set expected settings values.
	expected := input.KeyRepeatSettings{Enabled: true, Delay: 123, Interval: 1000}
	if err := input.SetKeyRepeatSettings(ctx, tconn, expected); err != nil {
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

func validateKeyRepeatSettings(ctx context.Context, a *arc.ARC, expected input.KeyRepeatSettings) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		delay, err := arcSecureSettingsValue(ctx, a, keyRepeatDelaySettingsName)
		if delay != expected.Delay {
			return errors.Wrapf(err, "key repeat delay %d doesn't match expected %d", delay, expected.Delay)
		}

		interval, err := arcSecureSettingsValue(ctx, a, keyRepeatIntervalSettingsName)
		if interval != expected.Interval {
			return errors.Wrapf(err, "key repeat interval %d doesn't match expected %d", interval, expected.Interval)
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second})
}
