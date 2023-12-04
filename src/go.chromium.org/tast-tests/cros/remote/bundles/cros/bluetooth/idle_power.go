// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IdlePower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure Bluetooth idle power consumption",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService", "tast.cros.power.MetricsService"},
		HardwareDeps: hwdep.D(hwdep.Battery()),
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "chromeUIDisabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// IdlePower tests power consumption when Bluetooth is off and when Bluetooth is on.
func IdlePower(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	interval := 5 * time.Minute // Power measurement interval in minutes

	// Disable Bluetooth
	fv.BluetoothService.Disable(ctx, &emptypb.Empty{})

	fv.StartPowerRecording(ctx)

	testing.ContextLog(ctx, "Keep BT off for ", interval)
	// GoBigSleepLint: sleep to keep BT off for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName()+".bt_off")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOff, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured power [W]: ", pOff)

	// Enable Bluetooth
	fv.BluetoothService.Enable(ctx, &emptypb.Empty{})

	fv.StartPowerRecording(ctx)

	testing.ContextLog(ctx, "Keep BT on for ", interval, " min")
	// GoBigSleepLint: sleep to keep BT on for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".bt_on")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOn, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured power [W]: ", pOn)

	s.Log("BT power consumption [W]: ", pOn-pOff)
	if pOn-pOff > 0.01 {
		s.Fatal("Bluetooth consumes more than 10mW power")
	}
}
