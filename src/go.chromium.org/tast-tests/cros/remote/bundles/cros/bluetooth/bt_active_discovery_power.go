// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast/core/testing"
)

type btActiveDiscoveryPowerTestCase struct {
	DeviceType cbt.DeviceType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BTActiveDiscoveryPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure active discovery power consumption",
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
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled_le_keyboard",
				Fixture:   "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &btActiveDiscoveryPowerTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
				},
			},
			{
				Name:              "floss_enabled_le_keyboard",
				Fixture:           "chromeUIDisabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &btActiveDiscoveryPowerTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
				},
			},
		},
	})
}

// BTActiveDiscoveryPower tests power during active discovery
func BTActiveDiscoveryPower(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	interval := 5 * time.Minute

	// Baseline idle power
	fv.StartPowerRecording(ctx)
	testing.ContextLog(ctx, "Keep idle for ", interval)
	// GoBigSleepLint: sleep to keep discovery for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName()+".idle")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pIdle, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power idle [W]: ", pIdle)
	}

	// Discovery
	testing.ContextLog(ctx, "Start discovery")
	if _, err = fv.BluetoothService.StartDiscovery(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to start discover: ", err)
	}

	fv.StartPowerRecording(ctx)
	testing.ContextLog(ctx, "Keep discovery for ", interval)
	// GoBigSleepLint: sleep to keep discovery for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".discov0")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pScan, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power discovery with 0 peer [W]: ", pScan)
	}

	testing.ContextLog(ctx, "Stop discovery")
	if _, err = fv.BluetoothService.StopDiscovery(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to stop discover: ", err)
	}

	btpeer := fv.BTPeers[0]
	tc := s.Param().(*btActiveDiscoveryPowerTestCase)

	// Emulate the desired device type with btpeer.
	testing.ContextLogf(ctx, "Configuring a btpeer as %q device", tc.DeviceType.String())
	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: tc.DeviceType,
	})
	if err != nil {
		s.Fatal("Failed to call NewEmulatedBTPeerDevice: ", err)
	}
	testing.ContextLogf(ctx, "Device %s is ready to pair", device.String())

	testing.ContextLog(ctx, "Start discovery")
	if _, err = fv.BluetoothService.StartDiscovery(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to start discover: ", err)
	}

	fv.StartPowerRecording(ctx)
	testing.ContextLog(ctx, "Keep discovery for ", interval)
	// GoBigSleepLint: sleep to keep discovery for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".discov1")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pScan1Peer, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power discovery with 1 peer [W]: ", pScan1Peer)
	}

	testing.ContextLog(ctx, "Stop discovery")
	if _, err = fv.BluetoothService.StopDiscovery(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to stop discover: ", err)
	}

	testing.ContextLog(ctx, "Discovery power consumption with 0 peer advertising: ", pScan-pIdle)
	testing.ContextLog(ctx, "Discovery power consumption with 1 peer advertising: ", pScan1Peer-pIdle)
}
