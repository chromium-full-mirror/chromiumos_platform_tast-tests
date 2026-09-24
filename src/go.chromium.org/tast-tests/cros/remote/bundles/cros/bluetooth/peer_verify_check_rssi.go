// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	pb "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PeerVerifyCheckRSSI,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that 4 btpeers are detected and their RSSI values via Floss are strictly greater than -70 dBm",
		Contacts: []string{
			"chromeos-bt-team@google.com",
		},
		BugComponent:    "b:167317", // ChromeOS > Platform > Connectivity > Bluetooth
		Attr:            []string{"group:bluetooth", "bluetooth_floss_flaky"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(4)},
		SoftwareDeps:    []string{"chrome"},
		ServiceDeps:     []string{"tast.cros.bluetooth.BluetoothService"},
		Fixture:         "chromeUIDisabledWith4BTPeersFlossEnabled",
		Timeout:         5 * time.Minute,
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
	})
}

// PeerVerifyCheckRSSI verifies that all 4 btpeers in the testbed can be
// discovered and report an RSSI strictly greater than -70 dBm over Floss.
func PeerVerifyCheckRSSI(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "number_of_btpeers",
		Unit:      "count",
		Direction: perf.BiggerIsBetter,
	}, float64(len(fv.BTPeers)))

	const (
		minRSSIThreshold = -70
		maxRSSIThreshold = 0
	)
	discoveryTimeout := durationpb.New(45 * time.Second)

	for i, peer := range fv.BTPeers {
		testing.ContextLogf(ctx, "Configuring btpeer %d as a mouse device", i)
		mouseDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, peer, &bluetooth.EmulatedBTPeerDeviceConfig{
			DeviceType: cbt.DeviceTypeMouse,
		})
		if err != nil {
			s.Errorf("Failed to configure btpeer %d as a mouse device: %v", i, err)
			continue
		}
		addr := mouseDevice.LocalBluetoothAddress()

		testing.ContextLogf(ctx, "Discovering and sampling RSSI for btpeer %d (%s)", i, addr)
		rssiResp, err := fv.BluetoothService.DiscoverDeviceAndSampleRSSI(ctx, &pb.DiscoverDeviceAndSampleRSSIRequest{
			DeviceAddress:    addr,
			DiscoveryTimeout: discoveryTimeout,
		})
		if err != nil {
			s.Errorf("Failed to discover and sample RSSI for btpeer %d (%s): %v", i, addr, err)
			continue
		}

		rssi := rssiResp.Rssi
		testing.ContextLogf(ctx, "RSSI for btpeer %d (%s): %d dBm", i, addr, rssi)

		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("rssi_peer_%d", i),
			Unit:      "dBm",
			Direction: perf.BiggerIsBetter,
		}, float64(rssi))

		if rssi <= minRSSIThreshold || rssi >= maxRSSIThreshold {
			s.Errorf("RSSI for btpeer %d (%s) is out of valid range (%d, %d) dBm: got %d dBm", i, addr, minRSSIThreshold, maxRSSIThreshold, rssi)
		}
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf metrics: ", err)
	}
}
