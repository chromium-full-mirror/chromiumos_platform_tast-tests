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
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type btPassiveScanPowerTestCase struct {
	DeviceType0 cbt.DeviceType
	DeviceType1 cbt.DeviceType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PassiveScanPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure passive scan power consumption with one peer paired and one peer advertising",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_2"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(2)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.power.DeviceSetupService",
			"tast.cros.power.RecorderService",
		},
		HardwareDeps: hwdep.D(hwdep.Battery()),
		Timeout:      25 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled_le_keyboard_le_mouse",
				Fixture:   "chromeUIDisabledWith2BTPeersPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &btPassiveScanPowerTestCase{
					DeviceType0: cbt.DeviceTypeLEKeyboard,
					DeviceType1: cbt.DeviceTypeLEMouse,
				},
			},
			{
				Name:              "floss_enabled_le_keyboard_le_mouse",
				Fixture:           "chromeUIDisabledWith2BTPeersPowerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &btPassiveScanPowerTestCase{
					DeviceType0: cbt.DeviceTypeLEKeyboard,
					DeviceType1: cbt.DeviceTypeLEMouse,
				},
			},
		},
	})
}

// PassiveScanPower tests power consumption when Bluetooth is enabled and doing passive scan.
func PassiveScanPower(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	btpeer0 := fv.BTPeers[0]
	btpeer1 := fv.BTPeers[1]
	tc := s.Param().(*btPassiveScanPowerTestCase)

	interval := 5 * time.Minute

	// Step 1: Get idle power.
	fv.StartPowerRecording(ctx)
	s.Log("Measuring power for Bluetooth idle for ", interval)
	// GoBigSleepLint: sleep to keep idle for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName()+".idle")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pIdle, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power idle [W]: ", pIdle)
	}

	// Step 2: Pair and disconnect LE device.
	// Emulate two btpeers with the specified device type.
	s.Logf("Configuring two btpeers as %q and %q device", tc.DeviceType0.String(), tc.DeviceType1.String())

	emulateBTPeer := func(btpeer *bluetooth.BtpeerClient, btType cbt.DeviceType) *bluetooth.EmulatedBTPeerDevice {
		newDev, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
			DeviceType: btType,
		})
		if err != nil {
			s.Fatal("Failed to emulate a BTPeer device: ", err)
		}
		s.Logf("Successfully emulating device: %s", newDev.String())
		return newDev
	}

	s.Log("Emulating btpeer0 for pairing")
	device0 := emulateBTPeer(btpeer0, tc.DeviceType0)
	s.Log("Emulating btpeer1 and keep it advertising")
	emulateBTPeer(btpeer1, tc.DeviceType1)

	// Attempt pairing BTPeer 0 with DUT.
	s.Log("Pairing btpeer0")
	if err := bluetoothutil.DiscoverAndPairDevice(ctx, fv.BluetoothService, device0.LocalBluetoothAddress(), device0.PinCode(), 45*time.Second); err != nil {
		s.Fatal("Failed to discover and pair btpeer0: ", err)
	}
	s.Log("Successfully paired btpeer0")

	// Check if peer is connected.
	resp, err := fv.BluetoothService.DeviceProperties(ctx, &bts.DevicePropertiesRequest{
		DeviceAddress: device0.LocalBluetoothAddress(),
	})
	if !resp.DeviceProperties.Connected {
		s.Fatal("Expected btpeer0 to be connected")
	}

	// Turn off connected BTPeer0's Bluetooth adapter to trigger DUT's passive scan.
	if err := device0.RPC().AdapterPowerOff(ctx); err != nil {
		s.Fatal("Failed to turn off btpeer0's adapter: ", err)
	}

	// Check if peer is disconnected
	resp, err = fv.BluetoothService.DeviceProperties(ctx, &bts.DevicePropertiesRequest{
		DeviceAddress: device0.LocalBluetoothAddress(),
	})
	if resp.DeviceProperties.Connected {
		s.Fatal("Expected btpeer0 to be disconnected")
	}

	fv.StartPowerRecording(ctx)

	s.Log("Measuring power for Bluetooth passive scan for ", interval)
	// GoBigSleepLint: sleep to keep discovery for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".scan")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pPassiveScan, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power for passive scan with 1 peer [W]: ", pPassiveScan)
	}

	// TODO: (b/301167351) Collect data to define baselines for test fail/pass.
	s.Log("Passive scan power consumption: ", pPassiveScan-pIdle)
}
