// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/testing"
)

// bluetoothSubPageURL is the URL of the Bluetooth sub-page within the OS Settings.
const bluetoothSubPageURL = "chrome://os-settings/bluetoothDevices"

func init() {
	testing.AddTest(&testing.Test{
		Func:           OpenBluetoothSettingsFromQuickSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that clicking the Settings button on the detailed Bluetooth page within the Quick Settings navigates to the Bluetooth Settings",
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

// OpenBluetoothSettingsFromQuickSettings tests that a user can successfully
// navigate through to the Bluetooth sub-page within the OS Settings from the
// Settings button in the Bluetooth detailed view within the Quick Settings.
func OpenBluetoothSettingsFromQuickSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(bluetooth.HasTconn).Tconn()

	if err := quicksettings.NavigateToBluetoothDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the detailed Bluetooth view: ", err)
	}

	if err := uiauto.New(tconn).LeftClick(quicksettings.BluetoothDetailedViewSettingsButton)(ctx); err != nil {
		s.Fatal("Failed to click the Bluetooth Settings button: ", err)
	}

	// Check if the Bluetooth sub-page within the OS Settings was opened.
	matcher := chrome.MatchTargetURL(bluetoothSubPageURL)
	conn, err := cr.NewConnForTarget(ctx, matcher)
	if err != nil {
		s.Fatal("Failed to open the Bluetooth settings: ", err)
	}
	defer conn.Close()
}
