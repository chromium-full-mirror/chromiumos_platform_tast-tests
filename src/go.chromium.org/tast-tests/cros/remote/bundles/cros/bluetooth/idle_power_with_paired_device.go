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
	sbt "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type idlePowerWithPairedDeviceTestCase struct {
	DeviceType cbt.DeviceType
	LLPrivacy  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: IdlePowerWithPairedDevice,
		Desc: "Measure Bluetooth with one paired device without user activities power consumption",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.power.DeviceSetupService",
			"tast.cros.power.RecorderService",
		},
		HardwareDeps:    hwdep.D(hwdep.Battery()),
		Timeout:         20 * time.Minute,
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{
			{
				Name:      "floss_disabled_le_keyboard",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
					LLPrivacy:  false,
				},
			},
			{
				Name:      "floss_disabled_bluetooth_audio",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeBluetoothAudio,
					LLPrivacy:  false,
				},
			},
			{
				Name:      "floss_enabled_le_keyboard",
				Fixture:   "chromeUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
					LLPrivacy:  false,
				},
			},
			{
				Name:      "floss_enabled_le_mouse",
				Fixture:   "chromeUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEMouse,
					LLPrivacy:  false,
				},
			},
			{
				Name:      "floss_enabled_mouse",
				Fixture:   "chromeUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeMouse,
					LLPrivacy:  false,
				},
			},
			{
				Name:      "floss_enabled_bluetooth_audio",
				Fixture:   "chromeUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeBluetoothAudio,
					LLPrivacy:  false,
				},
			},
			{
				Name:      "floss_enabled_llp_enabled_le_keyboard",
				Fixture:   "chromeUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
					LLPrivacy:  true,
				},
			},
			{
				Name:      "floss_enabled_llp_enabled_le_mouse",
				Fixture:   "chromeUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &idlePowerWithPairedDeviceTestCase{
					DeviceType: cbt.DeviceTypeLEMouse,
					LLPrivacy:  true,
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

	// Set LL privacy status.
	if _, err := fv.BluetoothService.SetLLPrivacy(ctx, &sbt.SetLLPrivacyRequest{Enabled: tc.LLPrivacy}); err != nil {
		s.Fatal("Failed to configure LL privacy: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Always try to disable LL privacy after test.
	defer func(ctx context.Context) {
		if _, err := fv.BluetoothService.SetLLPrivacy(ctx, &sbt.SetLLPrivacyRequest{Enabled: false}); err != nil {
			s.Fatal("Failed to disable LL privacy: ", err)
		}
	}(cleanupCtx)

	s.Log("Set bluetoothd config execution flags")
	if err := btpeer.ChameleondClient().BluetoothAudioDevice().ResetStack(ctx, tc.DeviceType.String()); err != nil {
		s.Fatal("Fail to reset stack: ", err)
	}

	// Emulate the desired device type with btpeer.
	testing.ContextLogf(ctx, "Configuring a btpeer as %q device", tc.DeviceType.String())
	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: tc.DeviceType,
	})
	if err != nil {
		s.Fatal("Failed to call NewEmulatedBTPeerDevice: ", err)
	}
	testing.ContextLogf(ctx, "Device %s is ready to pair", device.String())

	if tc.DeviceType == cbt.DeviceTypeBluetoothAudio {
		if err := bluetoothutil.ConfigureAudioDevice(ctx, device, cbt.AudioProfileA2DP, "", ""); err != nil {
			s.Fatal("Failed to config audio device: ", err)
		}
	}
	interval := 5 * time.Minute // Power measurement interval in minutes.

	if err := fv.PowerCooldown(ctx); err != nil {
		s.Fatal("Failed to cooldown for power measurement: ", err)
	}
	fv.StartPowerRecording(ctx)

	testing.ContextLog(ctx, "Keep BT on for ", interval)
	// GoBigSleepLint: sleep for measuring power consumption.
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName()+".bt_on")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOn, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured power for idle [W]: ", pOn)

	// Attempt pairing device with DUT.
	testing.ContextLogf(ctx, "Paring device %s", device.String())
	if err := bluetoothutil.DiscoverAndPairDevice(ctx, fv.BluetoothService, device.LocalBluetoothAddress(), device.PinCode(), 45*time.Second); err != nil {
		s.Fatalf("Failed to discover and pair device %s: %v", device.String(), err)
	}
	testing.ContextLogf(ctx, "Successfully paired device %s", device.String())

	if err := fv.PowerCooldown(ctx); err != nil {
		s.Fatal("Failed to cooldown for power measurement: ", err)
	}
	fv.StartPowerRecording(ctx)

	testing.ContextLog(ctx, "Keep BT on with 1 device paired for ", interval)
	// GoBigSleepLint: sleep for measuring power consumption.
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".bt_1peer")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	p1Peer, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured power with 1 device paired [W]: ", p1Peer)

	s.Log("BT power consumption [W]: ", p1Peer-pOn)
	if p1Peer-pOn > bluetoothutil.IdleWith1PeerPower {
		s.Fatal("Power consumption is over limit")
	}
}
