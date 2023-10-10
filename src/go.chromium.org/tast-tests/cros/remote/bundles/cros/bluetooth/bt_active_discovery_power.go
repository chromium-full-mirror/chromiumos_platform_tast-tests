// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
		TestBedDeps:  []string{tbdep.WorkingBluetoothPeers(1)},
		HardwareDeps: hwdep.D(hwdep.Battery()),
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
	s.Log("Keep idle for ", interval)
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

	// Use an invalid address to keep discovery running
	request := bts.DiscoverDeviceRequest{
		DeviceAddress:    "00:00:00:00:00:00",
		DiscoveryTimeout: durationpb.New(interval),
	}

	fv.StartPowerRecording(ctx)

	// Keep discovering for a non-existence target for interval seconds.
	s.Log("Keep discovering for ", interval)
	if _, err = fv.BluetoothService.DiscoverDevice(ctx, &request); err != nil {
		if strings.Contains(err.Error(), "failed to wait until bluetooth adapter discovered device") {
			s.Logf("Done discovering for %v for power measurement", interval)
		} else {
			s.Fatal("Unexpected discovery error: ", err)
		}
	}

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".discov0")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pScan, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power discovery with 0 peer [W]: ", pScan)
	}

	btpeer := fv.BTPeers[0]
	tc := s.Param().(*btActiveDiscoveryPowerTestCase)

	// Emulate the desired device type with btpeer.
	s.Logf("Configuring a btpeer as %q device", tc.DeviceType.String())
	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: tc.DeviceType,
	})
	if err != nil {
		s.Fatal("Failed to call NewEmulatedBTPeerDevice: ", err)
	}
	s.Logf("Device %s is ready to pair", device.String())

	fv.StartPowerRecording(ctx)
	s.Log("Keep discovering for ", interval)
	if _, err = fv.BluetoothService.DiscoverDevice(ctx, &request); err != nil {
		if strings.Contains(err.Error(), "failed to wait until bluetooth adapter discovered device") {
			s.Logf("Done discovering for %v for power measurement", interval)
		} else {
			s.Fatal("Unexpected discovery error: ", err)
		}
	}

	pResults, err = fv.StopPowerRecording(ctx, s.TestName()+".discov1")
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pScan1Peer, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err == nil {
		s.Log("Measured power discovery with 1 peer [W]: ", pScan1Peer)
	}

	s.Log("Discovery power consumption with 0 peer advertising: ", pScan-pIdle)
	s.Log("Discovery power consumption with 1 peer advertising: ", pScan1Peer-pIdle)
}
