// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	sbt "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IdlePowerWithLLP,
		Desc: "Measure Bluetooth idle power consumption with LL privacy enabled",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.power.DeviceSetupService",
			"tast.cros.power.RecorderService",
		},
		HardwareDeps:    hwdep.D(hwdep.Battery()),
		Timeout:         25 * time.Minute,
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{
			{
				Name:      "floss_enabled",
				Fixture:   "chromeUIDisabledStandalonePowerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
			},
		},
	})
}

// IdlePowerWithLLP tests power consumption when Bluetooth is off and when Bluetooth is on.
func IdlePowerWithLLP(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	interval := 10 * time.Minute // Power measurement interval in minutes

	// Disable LL Privacy.
	if _, err := fv.BluetoothService.SetLLPrivacy(ctx, &sbt.SetLLPrivacyRequest{Enabled: false}); err != nil {
		s.Fatal("Failed to disable LL privacy")
	}
	// Enable Bluetooth.
	if _, err := fv.BluetoothService.SetPowered(ctx, &sbt.SetPoweredRequest{Powered: true}); err != nil {
		s.Fatal("Failed to enable Bluetooth")
	}

	if err := fv.PowerCooldown(ctx); err != nil {
		s.Fatal("Failed to cooldown for power measurement: ", err)
	}
	fv.StartPowerRecording(ctx)

	s.Log("Keep Bluetooth on LL privacy off for ", interval)
	// GoBigSleepLint: sleep to keep LL privacy off for measuring power consumption.
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName()+".llp_off")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOff, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured power [W]: ", pOff)

	// Enable LL Privacy.
	if _, err := fv.BluetoothService.SetLLPrivacy(ctx, &sbt.SetLLPrivacyRequest{Enabled: true}); err != nil {
		s.Fatal("Failed to enable LL privacy")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Disable LL Privacy at the end of the test.
	defer func(ctx context.Context) {
		if _, err := fv.BluetoothService.SetLLPrivacy(ctx, &sbt.SetLLPrivacyRequest{Enabled: false}); err != nil {
			s.Error("Failed to disable LL privacy")
		}
	}(cleanupCtx)

	if err := fv.PowerCooldown(ctx); err != nil {
		s.Fatal("Failed to cooldown for power measurement: ", err)
	}
	fv.StartPowerRecording(ctx)

	s.Log("Keep Bluetooth on LL privacy on for ", interval, " min")
	// GoBigSleepLint: sleep to keep LL privacy on for measuring power consumption.
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".llp_on")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOn, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured power [W]: ", pOn)

	s.Log("LL privacy power consumption [W]: ", pOn-pOff)
	if pOn-pOff > bluetoothutil.IdlePower {
		s.Fatal("Power consumption is over limit")
	}
}
