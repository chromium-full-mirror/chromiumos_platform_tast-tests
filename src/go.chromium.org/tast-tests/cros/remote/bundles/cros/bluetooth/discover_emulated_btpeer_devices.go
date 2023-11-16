// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	pb "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/durationpb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DiscoverEmulatedBTPeerDevices,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests that btpeers can be set to emulate a type device and that the DUT can discover them as those devices",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jaredbennett@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr: []string{
			"group:bluetooth",
			"bluetooth_btpeers_2",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService"},
		TestBedDeps:  []string{tbdep.WorkingBluetoothPeers(2)},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeUIDisabledWith2BTPeersFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "chromeUIDisabledWith2BTPeersFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// DiscoverEmulatedBTPeerDevices tests that btpeers can be set to emulate a
// type device and that the DUT can discover them as those devices.
func DiscoverEmulatedBTPeerDevices(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	discoveryTimeout := durationpb.New(45 * time.Second)

	// Discover btpeer1 as a keyboard.
	testing.ContextLog(ctx, "Configuring btpeer1 as a keyboard device")
	keyboardDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeKeyboard,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer1 as a keyboard device: ", err)
	}
	if _, err := fv.BluetoothService.DiscoverDevice(ctx, &pb.DiscoverDeviceRequest{
		DeviceAddress:    keyboardDevice.LocalBluetoothAddress(),
		DiscoveryTimeout: discoveryTimeout,
	}); err != nil {
		s.Fatalf("DUT failed to discover btpeer1 as %s: %v", keyboardDevice.String(), err)
	}

	// Discover btpeer2 as a mouse.
	testing.ContextLog(ctx, "Configuring btpeer2 as a mouse device")
	mouseDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[1], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeMouse,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer2 as a mouse device: ", err)
	}
	if _, err := fv.BluetoothService.DiscoverDevice(ctx, &pb.DiscoverDeviceRequest{
		DeviceAddress:    mouseDevice.LocalBluetoothAddress(),
		DiscoveryTimeout: discoveryTimeout,
	}); err != nil {
		s.Fatalf("DUT failed to discover btpeer2 as %s: %v", mouseDevice.String(), err)
	}

	// Confirm that btpeer1 is also still discoverable as a keyboard, since both
	// peers should be usable at the same time.
	if _, err := fv.BluetoothService.DiscoverDevice(ctx, &pb.DiscoverDeviceRequest{
		DeviceAddress:    keyboardDevice.LocalBluetoothAddress(),
		DiscoveryTimeout: discoveryTimeout,
	}); err != nil {
		s.Fatalf("DUT failed to still discover btpeer1 as %s: %v", keyboardDevice.String(), err)
	}
}
