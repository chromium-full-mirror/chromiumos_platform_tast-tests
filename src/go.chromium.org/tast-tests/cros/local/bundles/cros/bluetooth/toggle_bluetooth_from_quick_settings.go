// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

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
		Func:           ToggleBluetoothFromQuickSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that Bluetooth can be enabled and disabled from within the Quick Settings",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth"},
		SoftwareDeps: []string{"chrome"},
		// Skip on form factors where a warning dialog may be shown when disabling Bluetooth due to all known HID devices being connected via Bluetooth.
		// These form factors are covered by another test see b/319492526
		HardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebase, hwdep.Chromebox, hwdep.Chromebit)),
		Params: []testing.Param{{
			Name:      "floss_disabled",
			Fixture:   "bluetoothEnabledWithBlueZ",
			ExtraAttr: []string{"bluetooth_sa"},
		}, {
			Name:              "floss_enabled",
			Fixture:           "bluetoothEnabledWithFloss",
			ExtraAttr:         []string{"bluetooth_floss"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}},
	})
}

// ToggleBluetoothFromQuickSettings tests that a user can successfully toggle
// the Bluetooth state using the Bluetooth feature pod icon button within the
// Quick Settings.
func ToggleBluetoothFromQuickSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(bluetooth.HasTconn).Tconn()
	bt := s.FixtValue().(bluetooth.HasBluetoothImpl).BluetoothImpl()

	if err := bt.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}

	ui := uiauto.New(tconn)

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show quick settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	// Dump UI tree while quick settings is open.
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	state := false
	const iterations = 20
	for i := 0; i < iterations; i++ {
		s.Logf("Toggling Bluetooth (iteration %d of %d)", i+1, iterations)

		if err := ui.LeftClick(quicksettings.FeatureTileBluetoothToggle)(ctx); err != nil {
			s.Fatal("Failed to click the Bluetooth feature pod icon button: ", err)
		}
		if err := bt.PollForAdapterState(ctx, state); err != nil {
			s.Fatal("Failed to toggle Bluetooth state: ", err)
		}
		state = !state
	}
}
