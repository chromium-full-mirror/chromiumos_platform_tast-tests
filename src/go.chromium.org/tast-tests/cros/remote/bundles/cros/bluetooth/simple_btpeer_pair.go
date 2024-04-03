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
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
)

type simpleBTPeerTestCase struct {
	DeviceType cbt.DeviceType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           SimpleBTPeerPair,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests pairing of classic and LE btpeers, with pairing done through dbus",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jaredbennett@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService"},
		Timeout:      1 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled__le_keyboard",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
				},
			},
			{
				Name:      "floss_disabled__le_mouse",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeLEMouse,
				},
			},
			{
				Name:      "floss_disabled__le_phone",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeLEPhone,
				},
			},
			{
				Name:      "floss_disabled__keyboard",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeKeyboard,
				},
			},
			{
				Name:      "floss_disabled__mouse",
				Fixture:   "chromeUIDisabledWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeMouse,
				},
			},
			{
				Name:              "floss_enabled__le_keyboard",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeLEKeyboard,
				},
			},
			{
				Name:              "floss_enabled__le_mouse",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeLEMouse,
				},
			},
			{
				Name:              "floss_enabled__le_phone",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeLEPhone,
				},
			},
			{
				Name:              "floss_enabled__keyboard",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeKeyboard,
				},
			},
			{
				Name:              "floss_enabled__mouse",
				Fixture:           "chromeUIDisabledWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val: &simpleBTPeerTestCase{
					DeviceType: cbt.DeviceTypeMouse,
				},
			},
		},
	})
}

// SimpleBTPeerPair tests pairing of classic and LE btpeers, with pairing done
// through dbus.
func SimpleBTPeerPair(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	btpeer := fv.BTPeers[0]
	tc := s.Param().(*simpleBTPeerTestCase)

	// Emulate the desired device type with btpeer.
	testing.ContextLogf(ctx, "Configuring btpeer as %q device", tc.DeviceType.String())
	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: tc.DeviceType,
	})
	if err != nil {
		s.Fatal("Failed to call NewEmulatedBTPeerDevice: ", err)
	}
	testing.ContextLogf(ctx, "Device %s ready to pair", device.String())

	// Attempt pairing device with DUT.
	testing.ContextLogf(ctx, "Paring device %s", device.String())
	if err := bluetoothutil.DiscoverAndPairDevice(ctx, fv.BluetoothService, device.LocalBluetoothAddress(), device.PinCode(), 45*time.Second); err != nil {
		s.Fatalf("Failed to discover and pair device %s: %v", device.String(), err)
	}
	testing.ContextLogf(ctx, "Successfully paired device %s", device.String())

	// Validate device properties from DUT end.
	testing.ContextLog(ctx, "Validating paired device properties")
	resp, err := fv.BluetoothService.DeviceProperties(ctx, &bts.DevicePropertiesRequest{
		DeviceAddress: device.LocalBluetoothAddress(),
	})
	if resp.DeviceProperties.Address != device.LocalBluetoothAddress() {
		s.Fatalf("Failed to verify device address; got: %q, want: %q", resp.DeviceProperties.Address, device.LocalBluetoothAddress())
	}
	if resp.DeviceProperties.Name != device.AdvertisedName() {
		s.Fatalf("Failed to verify device name; got: %q, want: %q", resp.DeviceProperties.Name, device.AdvertisedName())
	}
	if !resp.DeviceProperties.Paired {
		s.Fatalf("Expected device %s to be paired", device.String())
	}
	if !resp.DeviceProperties.Connected {
		s.Fatalf("Expected device %s to be connected", device.String())
	}
}
