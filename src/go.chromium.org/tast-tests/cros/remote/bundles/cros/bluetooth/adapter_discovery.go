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
	"google.golang.org/protobuf/types/known/durationpb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdapterDiscovery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the bluetooth adapter can discover a bluetooth device",
		Contacts: []string{
			"cros-connectivity@google.com",
		},
		BugComponent: "b:167317", // ChromeOS > Platform > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled_classic_mouse",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &bluetoothutil.DeviceTypeTestParam{
					DeviceType: cbt.DeviceTypeMouse,
				},
			},
			{
				Name:              "floss_enabled_classic_mouse",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &bluetoothutil.DeviceTypeTestParam{
					DeviceType: cbt.DeviceTypeMouse,
				},
			},
			{
				Name:      "floss_disabled_le_mouse",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &bluetoothutil.DeviceTypeTestParam{
					DeviceType: cbt.DeviceTypeLEMouse,
				},
			},
			{
				Name:              "floss_enabled_le_mouse",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &bluetoothutil.DeviceTypeTestParam{
					DeviceType: cbt.DeviceTypeLEMouse,
				},
			},
		},
	})
}

// AdapterDiscovery Tests that the bluetooth adapter can discover a bluetooth device.
//
// Configures btpeer1 as the device set in params and waits for it to be discovered.
func AdapterDiscovery(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	param := s.Param().(*bluetoothutil.DeviceTypeTestParam)

	// Configure btpeer1.
	s.Logf("Configuring btpeer1 as a %q device", param.DeviceType)
	btpeer1, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: param.DeviceType,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer1 as a %q device: %v", param.DeviceType, err)
	}
	s.Logf("Configured btpeer1 as %s", btpeer1.String())

	// Discover device and validate device name.
	const discoveryTimeout = 60 * time.Second
	s.Logf("Attempting to discover btpeer1 from DUT (%s timeout)", discoveryTimeout)
	if _, err := fv.BluetoothService.DiscoverDevice(ctx, &bts.DiscoverDeviceRequest{
		DeviceAddress:    btpeer1.LocalBluetoothAddress(),
		DiscoveryTimeout: durationpb.New(discoveryTimeout),
	}); err != nil {
		s.Fatalf("DUT failed to discover btpeer1 as %s: %v", btpeer1.String(), err)
	}
	s.Log("Validating discovered btpeer1 device name")
	devicePropResp, err := fv.BluetoothService.DeviceProperties(ctx, &bts.DevicePropertiesRequest{
		DeviceAddress: btpeer1.LocalBluetoothAddress(),
	})
	if err != nil {
		s.Fatal("Failed to get device properties after successful discovery: ", err)
	}
	observedDeviceProperties := devicePropResp.DeviceProperties
	if observedDeviceProperties.Name != btpeer1.AdvertisedName() {
		s.Fatalf("Expected discovered device name to be %q, got %q", btpeer1.AdvertisedName(), observedDeviceProperties.Name)
	}
	s.Logf("Successfully discovered btpeer1 with device name %q from DUT", observedDeviceProperties.Name)
}
