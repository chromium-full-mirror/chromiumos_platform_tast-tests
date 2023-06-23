// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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
		Params: []testing.Param{
			{
				Name:              "libassistant_v2",
				Fixture:           "assistantWithLibassistantV2QsRevampEnabled",
				ExtraSoftwareDeps: []string{"dlc"},
			},
			{
				Fixture: "assistantQsRevampEnabled",
			},
		},
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

	cleanup, err := quicksettings.Init(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to init quick settings")
	}
	defer cleanup()

	// Open the Settings window, where we can verify Bluetooth/Wifi status
	if err := apps.Launch(ctx, tconn, apps.Settings.ID); err != nil {
		s.Fatal("Failed to launch Settings app: ", err)
	}

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

		// Check if button in the Settings app UI updated to match the actual status.
		// The buttons don't update immediately, so we'll need to poll their statuses.
		// The "aria-pressed" htmlAttribute of the toggle buttons can be used to check the on/off status
		s.Log("Checking bluetooth toggle button status")
		ui := uiauto.New(tconn)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			bluetoothToggle := nodewith.Name("Bluetooth enable").Role(role.ToggleButton)
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(bluetoothToggle)(ctx); err != nil {
				testing.PollBreak(err)
			}

			info, err := ui.Info(ctx, bluetoothToggle)
			if err != nil {
				testing.PollBreak(err)
			}
			if info.HTMLAttributes["aria-pressed"] != strconv.FormatBool(status) {
				return errors.Errorf("bluetooth not toggled yet, aria-pressed is %v, expected %v",
					info.HTMLAttributes["aria-pressed"], status)
			}
			return nil
		}, nil); err != nil {
			s.Fatal("Bluetooth button (Settings app) was not toggled by the Assistant: ", err)
		}

		// Check Bluetooth quick setting tile as well.
		s.Log("Checking bluetooth status in Quick Settings")
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
