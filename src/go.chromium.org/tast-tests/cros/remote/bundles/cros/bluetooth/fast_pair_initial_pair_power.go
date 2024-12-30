// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/base64"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FastPairInitialPairPower,
		Desc: "Tests the power consumption for Fast Pair initial pairing scenario and while Fast Pair is enabled and device is advertising",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1133283", // ChromeOS > Software > System Services > Cross Device > Fast Pair
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(bluetooth.FastPairHardwareDep, hwdep.Battery()),
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.power.DeviceSetupService",
			"tast.cros.power.RecorderService",
		},
		Timeout: 20 * time.Minute,
		Vars:    []string{bluetooth.TestVarFastPairAntispoofingKeyPem},
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeLoggedInAsUserWithFastPairAnd1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:      "floss_enabled",
				Fixture:   "chromeLoggedInAsUserWithFastPairAnd1BTPeerPowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
			},
		},
	})
}

// FastPairInitialPairPower tests the Fast Pair initial pairing scenario with power measurement.
func FastPairInitialPairPower(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	interval := 5 * time.Minute // Power measurement interval

	// Parse antispoofing key pem from test var.
	antispoofingKeyPemBase64 := s.RequiredVar(bluetooth.TestVarFastPairAntispoofingKeyPem)
	antispoofingKeyPem, err := base64.StdEncoding.DecodeString(antispoofingKeyPemBase64)
	if err != nil {
		s.Fatalf("Failed to decode %q from base64 string: %v", bluetooth.TestVarFastPairAntispoofingKeyPem, err)
	}

	// Record a baseline power
	if err := fv.PowerCooldown(ctx); err != nil {
		s.Fatal("Failed to cooldown for power measurement: ", err)
	}
	fv.StartPowerRecording(ctx)

	s.Log("Measuring power with Fast Pair enabled, but no devices advertising for ", interval)
	// GoBigSleepLint: sleep to measure power consumption
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName()+".idle")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pWait, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	// TODO: (b/301167351) Collect data to define baselines for test fail/pass.
	s.Log("Power consumption for Fast Pair without a peer nearby [W]: ", pWait)

	// Record FP power
	if err := fv.PowerCooldown(ctx); err != nil {
		s.Fatal("Failed to cooldown for power measurement: ", err)
	}
	// Configure btpeer as a Fast Pair device.
	fastPairDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeLEFastPair,
	})
	if err != nil {
		s.Error("Failed to configure the btpeer as a Fast Pair device: ", err)
	}
	if err := fastPairDevice.RPCFastPair().SetAntispoofingKeyPem(ctx, antispoofingKeyPem); err != nil {
		s.Error("Failed to set antispoofing key pem on Fast Pair btpeer: ", err)
	}

	fv.StartPowerRecording(ctx)
	s.Log("Measuring power with Fast Pair enabled, and one device advertising for ", interval)
	// GoBigSleepLint: sleep to measure power consumption
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".FastPair")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pPair, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	// TODO: (b/301167351) Collect data to define baselines for test fail/pass.
	s.Log("Power consumption for Fast Pair with one peer nearby [W]: ", pPair)
}
