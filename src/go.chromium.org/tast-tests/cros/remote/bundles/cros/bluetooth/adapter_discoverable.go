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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdapterDiscoverable,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the DUT can be discovered from a bluetooth device",
		Contacts: []string{
			"cros-connectivity@google.com",
		},
		BugComponent: "b:167317", // ChromeOS > Platform > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// AdapterDiscoverable verifies that the dut can be discovered from a bluetooth
// device.
//
// Configures btpeer1 as a classic mouse, makes the dut discoverable, then
// ensures that the btpeer can discover the dut.
func AdapterDiscoverable(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	// Get the dut address.
	s.Log("Retrieving dut bluetooth address from adapter")
	addressResponse, err := fv.BluetoothService.Address(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get address of dut: ", err)
	}
	s.Logf("Got dut bluetooth address as %q", addressResponse.AdapterAddress)

	// Configure btpeer1.
	deviceType := cbt.DeviceTypeMouse
	s.Logf("Configuring btpeer1 as a %q device", deviceType)
	btpeer1, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: deviceType,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer1 as a %q device: %v", deviceType, err)
	}
	s.Logf("Configured btpeer1 as %s", btpeer1.String())

	// Confirm that dut is not discoverable.
	s.Log("Confirming dut is not discoverable")
	isDiscoverableResponse, err := fv.BluetoothService.IsDiscoverable(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to check if dut is discoverable: ", err)
	}
	if isDiscoverableResponse.IsDiscoverable {
		s.Fatal("BluetoothService.IsDiscoverable returned true before setting dut as discoverable with BluetoothService.SetDiscoverable")
	}

	// Confirm that btpeer1 cannot discover dut yet.
	s.Log("Confirming btpeer1 cannot discover dut yet")
	if err := btpeer1.RPC().Discover(ctx, addressResponse.AdapterAddress); err == nil {
		s.Fatal("Btpeer1 was able to discover dut before calling BluetoothService.SetDiscoverable")
	}

	// Make dut discoverable.
	s.Log("Setting dut as discoverable via adapter")
	if _, err := fv.BluetoothService.SetDiscoverable(ctx, &bts.SetDiscoverableRequest{
		Discoverable: true,
	}); err != nil {
		s.Fatal("Failed to make dut discoverable: ", err)
	}

	// Double-check that SetDiscoverable actually worked and that state is
	// reflected in IsDiscoverable.
	s.Log("Confirming dut is discoverable")
	isDiscoverableResponse, err = fv.BluetoothService.IsDiscoverable(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to check if dut is discoverable: ", err)
	}
	if !isDiscoverableResponse.IsDiscoverable {
		s.Fatal("BluetoothService.IsDiscoverable returned false after setting dut as discoverable with BluetoothService.SetDiscoverable")
	}

	// Poll for dut discovery from btpeer.
	const discoverTimeout = 1 * time.Minute
	const discoverInterval = time.Second
	s.Logf("Waiting for btpeer1 to discover dut at address %q (%s timeout, %s interval)", addressResponse.AdapterAddress, discoverTimeout, discoverInterval)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		err := btpeer1.RPC().Discover(ctx, addressResponse.AdapterAddress)
		if err != nil {
			return errors.Wrap(err, "btpeer1 failed to discover dut")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  discoverTimeout,
		Interval: discoverInterval,
	}); err != nil {
		s.Fatalf("Failed to discover dut at address %q from btpeer1: %v", addressResponse.AdapterAddress, err)
	}
	s.Log("Successfully discovered dut from btpeer1")
}
