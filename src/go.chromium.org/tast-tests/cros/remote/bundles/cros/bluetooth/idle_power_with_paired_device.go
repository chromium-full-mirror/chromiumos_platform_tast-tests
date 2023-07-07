// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	"go.chromium.org/tast/core/testing"
)

type idlePowerWithPairedDeviceTestCase struct {
	DeviceType cbt.DeviceType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         IdlePowerWithPairedDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure Bluetooth with one paired device without user activities power consumption",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr: []string{
			"group:bluetooth",
			"bluetooth_btpeers_1",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService", "tast.cros.power.MetricsService"},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled_le_keyboard",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
				},
			},
			{
				Name:      "floss_disabled_bluetooth_audio",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeBluetoothAudio,
				},
			},
			{
				Name:      "floss_enabled_le_keyboard",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
				},
			},
			{
				Name:      "floss_enabled_bluetooth_audio",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeBluetoothAudio,
				},
			},
		},
	})
}

// IdlePowerWithPairedDevice tests power consumption when Bluetooth is on and with one device paired.
func IdlePowerWithPairedDevice(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	btpeer := fv.BTPeers[0]
	tc := s.Param().(*idlePowerWithPairedDeviceTestCase)

	// Emulate the desired device type with btpeer.
	testing.ContextLogf(ctx, "Configuring a btpeer as %q device", tc.DeviceType.String())
	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: tc.DeviceType,
	})
	if err != nil {
		s.Fatal("Failed to call NewEmulatedBTPeerDevice: ", err)
	}
	testing.ContextLogf(ctx, "Device %s is ready to pair", device.String())

	interval := 5 * time.Minute // Power measurement interval in minutes

	// Attempt pairing device with DUT.
	testing.ContextLogf(ctx, "Paring device %s", device.String())
	if err := bluetoothutil.DiscoverAndPairDevice(ctx, fv.BluetoothService, device.LocalBluetoothAddress(), device.PinCode(), 45*time.Second); err != nil {
		s.Fatalf("Failed to discover and pair device %s: %v", device.String(), err)
	}
	testing.ContextLogf(ctx, "Successfully paired device %s", device.String())

	fv.StartPowerRecording(ctx)

	testing.ContextLog(ctx, "Keep BT on for ", interval)
	// GoBigSleepLint: sleep for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName())
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOn, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	// TODO: (b/301167351) Collect data to define baselines for test fail/pass.
	s.Log("Measured power [W]: ", pOn)
}
