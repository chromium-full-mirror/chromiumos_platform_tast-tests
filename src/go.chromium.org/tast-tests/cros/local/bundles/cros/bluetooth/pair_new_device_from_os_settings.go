// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PairNewDeviceFromOSSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that the pairing dialog can be opened from the OS Settings",
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
		}},
	})
}

// PairNewDeviceFromOSSettings tests that a user can successfully open the
// pairing dialog from the "Pair new device" button on the OS Settings page.
func PairNewDeviceFromOSSettings(ctx context.Context, s *testing.State) {
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

	if err := uiauto.Combine("Open the \"Pair new device\" dialog",
		ui.LeftClick(ossettings.BluetoothPairNewDeviceButton),
		ui.WaitUntilExists(ossettings.BluetoothPairNewDeviceModal),
	)(ctx); err != nil {
		s.Fatal("Failed to open the \"Pair new device\" dialog: ", err)
	}
}
