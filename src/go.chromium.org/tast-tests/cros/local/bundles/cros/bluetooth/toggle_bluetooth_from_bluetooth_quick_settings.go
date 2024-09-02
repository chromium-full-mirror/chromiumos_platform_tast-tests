// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ToggleBluetoothFromBluetoothQuickSettings,
		Desc: "Checks that Bluetooth can be enabled and disabled from within the Bluetooth Quick Settings",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:bluetooth", "bluetooth_sa", "bluetooth_floss"},
		SoftwareDeps:   []string{"chrome"},
		TestBedDeps:    []string{tbdep.BluetoothStateNormal},
		// Skip on form factors where a warning dialog may be shown when disabling Bluetooth due to all known HID devices being connected via Bluetooth.
		// These form factors are covered by another test see b/319492526
		HardwareDeps:    hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebase, hwdep.Chromebox, hwdep.Chromebit)),
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{{
			Name:    "logged_in",
			Fixture: "bluetoothEnabledWithFloss",
		}, {
			Name:    "oobe",
			Fixture: "bluetoothEnabledInOobeWithFloss",
		}},
	})
}

// ToggleBluetoothFromBluetoothQuickSettings tests that a user can successfully
// toggle the Bluetooth state using the toggle in the detailed Bluetooth view
// within the Quick Settings.
func ToggleBluetoothFromBluetoothQuickSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(bluetooth.HasTconn).Tconn()

	if err := quicksettings.NavigateToBluetoothDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the detailed Bluetooth view: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	// Dump UI tree while quick settings is open.
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	bt := s.FixtValue().(bluetooth.HasBluetoothImpl).BluetoothImpl()

	if err := bt.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}

	if err := bt.PollForEnabled(ctx); err != nil {
		s.Fatal("Expected Bluetooth to be enabled: ", err)
	}

	ui := uiauto.New(tconn)

	state := false
	const iterations = 20
	for i := 0; i < iterations; i++ {
		s.Logf("Toggling Bluetooth (iteration %d of %d)", i+1, iterations)

		if err := ui.LeftClick(quicksettings.BluetoothDetailedViewToggleButton)(ctx); err != nil {
			s.Fatal("Failed to click the Bluetooth toggle: ", err)
		}
		if err := bt.PollForAdapterState(ctx, state); err != nil {
			s.Fatal("Failed to toggle Bluetooth state: ", err)
		}
		state = !state
	}
}
