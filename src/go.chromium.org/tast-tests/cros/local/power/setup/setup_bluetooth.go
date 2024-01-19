// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/floss"
	"go.chromium.org/tast/core/testing"
)

func disableBluezAdapter(ctx context.Context) (CleanupCallback, error) {
	if adapters, err := bluez.Adapters(ctx); err == nil && len(adapters) == 0 {
		testing.ContextLog(ctx, "Bluetooth adapter is missing")
		return func(ctx context.Context) error { return nil }, nil
	}
	if state, err := bluez.IsEnabled(ctx); err == nil && state == false {
		testing.ContextLog(ctx, "Bluetooth adapter is disabled")
		return func(ctx context.Context) error { return nil }, nil
	}
	testing.ContextLog(ctx, "Disabling the bluez adapter")
	if err := bluez.Disable(ctx); err != nil {
		return func(ctx context.Context) error { return nil }, err
	}
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Enabling the bluez adapter")
		return bluez.Enable(ctx)
	}, nil
}

func disableFlossAdapter(ctx context.Context) (CleanupCallback, error) {
	if state, err := floss.GetFlossEnabled(ctx); err == nil && state == false {
		testing.ContextLog(ctx, "Bluetooth adapter is missing or disabled")
		return func(ctx context.Context) error { return nil }, nil
	}
	testing.ContextLog(ctx, "Disabling the floss adapter")
	if err := floss.SetFlossEnabled(ctx, false); err != nil {
		return func(ctx context.Context) error { return nil }, err
	}
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Enabling the floss adapter")
		return floss.SetFlossEnabled(ctx, true)
	}, nil
}

// DisableBluetooth disables bluetooth service.
// Check if the floss service is enabled, if yes disable it.
// If not, then bluez service is enabled, disable it instead.
func DisableBluetooth(ctx context.Context) (CleanupCallback, error) {
	return Nested(ctx, "disable bluetooth", func(s *Setup) error {
		shortCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		isFlossEnabled, err := floss.GetFlossEnabled(shortCtx)
		if err != nil {
			// If the floss manager cannot be found, it's equivalent to floss being disabled.
			if !strings.Contains(err.Error(), `failed to connect to service "org.chromium.bluetooth.Manager"`) {
				return err
			}
			testing.ContextLog(ctx, "Failed to get the enabled state of floss: ", err)
			isFlossEnabled = false
		}
		if isFlossEnabled {
			s.Add(disableFlossAdapter(ctx))
		} else {
			s.Add(disableBluezAdapter(ctx))
		}
		return nil
	})
}
