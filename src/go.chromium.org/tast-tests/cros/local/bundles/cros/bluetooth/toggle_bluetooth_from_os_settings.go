// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ToggleBluetoothFromOSSettings,
		Desc: "Checks that Bluetooth can be enabled and disabled from the OS Settings",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:bluetooth", "bluetooth_sa", "bluetooth_floss"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "bluetoothEnabledWithFloss",
		TestBedDeps:    []string{tbdep.BluetoothStateNormal},
		// Skip on form factors where a warning dialog may be shown when disabling Bluetooth due to all known HID devices being connected via Bluetooth.
		// These form factors are covered by another test see b/319492526
		HardwareDeps:    hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebase, hwdep.Chromebox, hwdep.Chromebit)),
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
	})
}

// ToggleBluetoothFromOSSettings tests that a user can successfully toggle the
// Bluetooth state using the Bluetooth toggle on the OS Settings page.
func ToggleBluetoothFromOSSettings(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(bluetooth.HasTconn).Tconn()

	app, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Bluetooth)
	defer app.Close(ctx)

	if err != nil {
		s.Fatal("Failed to launch Bluetooth page in OS Settings: ", err)
	}

	bt := s.FixtValue().(bluetooth.HasBluetoothImpl).BluetoothImpl()

	if err := bt.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}

	ui := uiauto.New(tconn)

	if err := ui.WaitUntilExists(ossettings.OSSettingsBluetoothToggleButton)(ctx); err != nil {
		s.Fatal("Failed to find the Bluetooth toggle: ", err)
	}

	state := false
	const iterations = 20
	for i := 0; i < iterations; i++ {
		s.Logf("Toggling Bluetooth (iteration %d of %d)", i+1, iterations)

		if err := ui.LeftClick(ossettings.OSSettingsBluetoothToggleButton)(ctx); err != nil {
			s.Fatal("Failed to click the Bluetooth toggle: ", err)
		}
		if err := bt.PollForAdapterState(ctx, state); err != nil {
			s.Fatal("Failed to toggle Bluetooth state: ", err)
		}
		state = !state
	}
}
