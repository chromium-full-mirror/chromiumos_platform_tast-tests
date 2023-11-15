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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceBattery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that the battery information can be correctly presented to the end user",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"alfredyu@cienet.com",
			"cienet-development@googlegroups.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
		},
		TestBedDeps: []string{tbdep.WorkingBluetoothPeers(1)},
		Params: []testing.Param{
			{
				Name:      "floss_disabled__le_keyboard",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       cbt.DeviceTypeLEKeyboard,
			}, {
				Name:      "floss_disabled__le_mouse",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       cbt.DeviceTypeLEMouse,
			}, {
				Name:              "floss_enabled__le_keyboard",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               cbt.DeviceTypeLEKeyboard,
			}, {
				Name:              "floss_enabled__le_mouse",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               cbt.DeviceTypeLEMouse,
			},
		},
	})
}

// DeviceBattery tests that the battery information can be correctly presented to the end user.
func DeviceBattery(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: s.Param().(cbt.DeviceType),
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer as a %s device: %s", s.Param().(cbt.DeviceType), err)
	}

	// Forgetting Bluetooth device requires series of UI operations which could take a while.
	const forgetBTDeviceTimeout = time.Minute

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, forgetBTDeviceTimeout)
	defer cancel()

	if _, err := fv.BluetoothUIService.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: device.AdvertisedName(),
	}); err != nil {
		s.Fatalf("Failed to pair the Bluetooth device %q with quick settings: %v", device.String(), err)
	}
	defer fv.BluetoothUIService.ForgetBluetoothDevice(cleanupCtx, &bts.ForgetBluetoothDeviceRequest{DeviceName: device.AdvertisedName()})

	// The Bluetooth devices page should report the battery level of the target device.
	resp, err := fv.BluetoothUIService.CollectDeviceList(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatalf("Failed to verify the device %q information is displayed in Setting: %v", device.String(), err)
	}
	for _, d := range resp.Devices {
		// Expecting the emulated BT device has the battery level displayed.
		if d.Name == device.AdvertisedName() && len(d.BatteryInformation) == 0 {
			s.Fatal("Failed to verify Bluetooth devices page reports the battery info of the target BT device correctly: battery level does not available")
		}
	}

	// The device detail page should report its battery level as well.
	if resp, err := fv.BluetoothUIService.BluetoothDeviceDetail(ctx, &bts.BluetoothDeviceDetailRequest{
		Name: device.AdvertisedName(),
	}); err != nil {
		s.Fatal("Failed to verify Bluetooth device detail page reports the battery info correctly: ", err)
	} else if len(resp.Device.BatteryInformation) == 0 {
		s.Fatal("Failed to verify Bluetooth device detail page reports the battery info correctly: battery level does not available")
	}

	// Disconnect from the BT device (by powering it off) to verify that the device
	// detail page should not report the battery level of a disconnected BT device.
	if err := device.RPC().AdapterPowerOff(ctx); err != nil {
		s.Fatal("Failed to power off the device: ", err)
	}

	if _, err := fv.BluetoothService.WaitForConnectState(ctx, &bts.WaitForConnectStateRequest{
		DeviceAddress:        device.LocalBluetoothAddress(),
		ExpectedConnectState: false, /* wait for the device to be disconnected */
	}); err != nil {
		s.Fatal("Failed to wait for the target BT device to be disconnected: ", err)
	}

	if resp, err := fv.BluetoothUIService.BluetoothDeviceDetail(ctx, &bts.BluetoothDeviceDetailRequest{
		Name: device.AdvertisedName(),
	}); err != nil {
		s.Fatal("Failed to verify Bluetooth device detail page reports the battery info correctly: ", err)
	} else if len(resp.Device.BatteryInformation) != 0 {
		s.Fatal("Failed to verify Bluetooth device detail page reports the battery info correctly: the battery info is still presented")
	}
}
