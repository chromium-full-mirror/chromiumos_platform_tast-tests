// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bluetooth/mojo"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ToggleBluetoothUsingMojo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Bluetooth can be enabled and disabled using Mojo API",
		Contacts: []string{
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"cros-connectivity@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_flaky"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:      "floss_disabled",
			Fixture:   "bluetoothMojoJSObjectWithBlueZ",
			ExtraAttr: []string{"bluetooth_flaky"},
		}, {
			Name:              "floss_enabled",
			Fixture:           "bluetoothMojoJSObjectWithFloss",
			ExtraAttr:         []string{"bluetooth_floss_flaky"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}},
	})
}

// ToggleBluetoothUsingMojo verifies that bluetooth can be toggled using the CrosBluetoothConfig mojo API.
func ToggleBluetoothUsingMojo(ctx context.Context, s *testing.State) {
	bt := s.FixtValue().(mojo.HasBluetoothImpl).BluetoothImpl()
	js := s.FixtValue().(mojo.HasJSObject).JSObject()

	// Use an even number of iterations so that we end with bluetooth enabled.
	const iterations = 6
	for i := 0; i < iterations; i++ {

		var isEnabled bool
		var expectedState mojo.BluetoothSystemState

		if i%2 == 0 {
			isEnabled = false
			expectedState = mojo.Disabled
		} else {
			isEnabled = true
			expectedState = mojo.Enabled
		}

		s.Logf("Toggling Bluetooth state to %t (iteration %d of %d)", isEnabled, i+1, iterations)

		if err := mojo.SetBluetoothEnabledState(ctx, *js, isEnabled); err != nil {
			s.Fatal("Failed to toggle Bluetooth state via mojo: ", err)
		}

		if err := bt.PollForAdapterState(ctx, isEnabled); err != nil {
			s.Fatal("Bluetooth state not as expected: ", err)
		}

		if err := mojo.PollForBluetoothSystemState(ctx, *js, expectedState); err != nil {
			s.Fatal("Failed to get SystemProperties: ", err)
		}
	}
}
