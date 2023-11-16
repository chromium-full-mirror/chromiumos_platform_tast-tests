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
)

// bluetoothPairingDialogURL is the URL of the Bluetooth pairing dialog.
const bluetoothPairingDialogURL = "chrome://bluetooth-pairing/"

func init() {
	testing.AddTest(&testing.Test{
		Func:           PairNewDeviceFromBluetoothQuickSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that the pairing dialog can be opened from within the Bluetooth Quick Settings",
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
			ExtraAttr: []string{"bluetooth_sa"},
		}, {
			Name:              "floss_enabled",
			Fixture:           "bluetoothEnabledWithFloss",
			ExtraAttr:         []string{"bluetooth_floss"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}, {
			Name:      "floss_disabled_oobe",
			Fixture:   "bluetoothEnabledInOobeWithBlueZ",
			ExtraAttr: []string{"bluetooth_sa"},
		}, {
			Name:              "floss_enabled_oobe",
			Fixture:           "bluetoothEnabledInOobeWithFloss",
			ExtraAttr:         []string{"bluetooth_floss"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}},
	})
}

// PairNewDeviceFromBluetoothQuickSettings tests that a user can successfully
// open the pairing dialog from the "Pair new device" button in the detailed
// Bluetooth view within the Quick Settings.
func PairNewDeviceFromBluetoothQuickSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(bluetooth.HasTconn).Tconn()

	if err := quicksettings.NavigateToBluetoothDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the detailed Bluetooth view: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	bt := s.FixtValue().(bluetooth.HasBluetoothImpl).BluetoothImpl()

	if err := bt.PollForEnabled(ctx); err != nil {
		s.Fatal("Expected Bluetooth to be enabled: ", err)
	}

	if err := uiauto.New(tconn).LeftClick(quicksettings.BluetoothDetailedViewPairNewDeviceButton)(ctx); err != nil {
		s.Fatal("Failed to click the \"Pair new device\" button: ", err)
	}

	// Check if the Bluetooth pairing dialog was opened.
	matcher := chrome.MatchTargetURL(bluetoothPairingDialogURL)
	conn, err := cr.NewConnForTarget(ctx, matcher)
	if err != nil {
		s.Fatal("Failed to open the Bluetooth pairing dialog: ", err)
	}
	defer conn.Close()
}
