// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PairBluetoothDeviceWithUI,
		Desc: "Tests that we can pair a Bluetooth device with the UI",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"chromeos-connectivity-engprod@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > Fundamentals > Device Enablement > Connectivity > Bluetooth
		LifeCycleStage:  testing.LifeCycleOwnerMonitored,
		Attr:            []string{"group:bluetooth", "bluetooth_floss", "group:release-health", "release-health_bt"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps:    []string{"chrome"},
		ServiceDeps:     []string{"tast.cros.bluetooth.BluetoothUIService"},
		Timeout:         90 * time.Second,
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{
			{
				Name:    "floss_enabled__le_mouse",
				Fixture: "chromeLoggedInWith1BTPeerFlossEnabled",
				Val:     cbt.DeviceTypeLEMouse,
			},
		},
	})
}

// PairBluetoothDeviceWithUI tests that Bluetooth devices are able to pair with
// the Quick Settings UI.
func PairBluetoothDeviceWithUI(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	emulatedDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0],
		&bluetooth.EmulatedBTPeerDeviceConfig{DeviceType: s.Param().(cbt.DeviceType)})
	if err != nil {
		s.Fatal("Failed to emulate the device type: ", err)
	}

	if _, err := fv.BluetoothUIService.PairDeviceWithQuickSettings(
		ctx, &bts.PairDeviceWithQuickSettingsRequest{
			AdvertisedName: emulatedDevice.AdvertisedName(),
		}); err != nil {
		s.Fatal("Failed to pair device through the Quick Settings: ", err)
	}
}
