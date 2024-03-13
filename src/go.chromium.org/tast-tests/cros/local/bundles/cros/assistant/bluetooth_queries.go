// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BluetoothQueries,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests toggling Bluetooth using Assistant queries",
		Contacts:     []string{"assistive-eng@google.com"},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "assistantWithGaia",
	})
}

// BluetoothQueries tests that Assistant queries can be used to toggle Bluetooth on/off
func BluetoothQueries(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(
		ctx, s.OutDir(), s.HasError, tconn, "bluetooth_queries")

	// Turn settings on, off, and on again to ensure they can be enabled and disabled, regardless of starting state
	statuses := []bool{true, false, true}
	var onOff string
	for _, status := range statuses {
		if status {
			onOff = "on"
		} else {
			onOff = "off"
		}

		s.Log("Turning bluetooth ", onOff)
		// assistant.SendTextQuery sometimes times out after the assistant UI is closed,
		// so poll to ensure the queries go through.
		// TODO(crbug/1080363): remove polling.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			_, err := assistant.SendTextQuery(ctx, tconn, fmt.Sprintf("turn bluetooth %v", onOff))
			return err
		}, nil); err != nil {
			s.Fatal("Failed to get Assistant bluetooth query response: ", err)
		}

		// Check if Bluetooth is updated at system level via dbus.
		s.Log("Checking Bluetooth status using dbus")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if enabled, err := bluetoothEnabled(ctx); err != nil {
				return testing.PollBreak(err)
			} else if enabled != status {
				return errors.Wrapf(err, "incorrect bluetooth state (expected: %v, actual: %v", status, enabled)
			}
			return nil
		}, nil); err != nil {
			s.Fatal("Failed checking bluetooth status via dbus: ", err)
		}

		// Check if UI is updated correctly via Quick Settings.
		s.Log("Checking Bluetooth status in Quick Settings")
		if btStatus, err := quicksettings.BluetoothEnabled(ctx, tconn); err != nil {
			s.Fatal("Failed to get Bluetooth quick setting status: ", err)
		} else if btStatus != status {
			s.Fatal("Bluetooth quick setting tile was not toggled by the Assistant")
		}
	}
}

// bluetoothEnabled checks if the bluetooth adapter is enabled using dbus
func bluetoothEnabled(ctx context.Context) (bool, error) {
	adapters, err := bluez.Adapters(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get bluetooth adapters")
	}
	if len(adapters) != 1 {
		return false, errors.Errorf("unexpected Bluetooth adapters count; got %d, want 1", len(adapters))
	}
	adapter := adapters[0]
	return adapter.Powered(ctx)
}
