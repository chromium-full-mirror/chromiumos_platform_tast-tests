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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ToggleBluetoothFromQuickSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Bluetooth can be enabled and disabled from within the Quick Settings",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:      "floss_disabled",
			Fixture:   "bluetoothEnabledWithBlueZ",
			ExtraAttr: []string{"bluetooth_flaky"},
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
		if state {
			// Enabling bluetooth takes us to the bluetooth detailed page.
			// Navigate back to the main page.
			previousMenu := nodewith.Role(role.Button).NameContaining("Previous menu")
			if err := uiauto.Combine("navigate back to main page",
				ui.WaitUntilExists(previousMenu),
				ui.DoDefault(previousMenu))(ctx); err != nil {
				s.Fatal("Failed to click on previous menu button: ", err)
			}
		}
		state = !state
	}
}
