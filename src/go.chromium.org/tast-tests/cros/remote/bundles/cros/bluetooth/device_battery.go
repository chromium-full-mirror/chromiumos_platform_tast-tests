// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DeviceBattery,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Test that the battery information can be correctly presented to the end user",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:bluetooth"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
		},
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
		s.Fatalf("Failed to configure btpeer as a %s device: %v", s.Param().(cbt.DeviceType), err)
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
	targetDevice, err := findPairedDevice(ctx, fv.BluetoothUIService, func(d *bts.Device) bool { return d.GetName() == device.AdvertisedName() })
	if err != nil {
		s.Fatal("Failed to found the expected Bluetooth Device from Bluetooth devices page: ", err)
	}
	// Expecting the emulated BT device has the battery level displayed.
	if targetDevice.GetBattery() == nil {
		s.Fatal("Failed to verify Bluetooth devices page reports the battery info of the target BT device correctly: battery level does not available")
	}

	// The device detail page should report its battery level as well.
	if _, err := fv.BluetoothUIService.BluetoothDeviceDetail(ctx, &bts.BluetoothDeviceDetailRequest{
		Name:        device.AdvertisedName(),
		MatchOption: bts.BluetoothDeviceDetailRequest_MATCH_OPTION_REPORT_BATTERY.Enum(),
	}); err != nil {
		s.Fatal("Failed to verify Bluetooth device detail page reports the battery info correctly: battery level does not available: ", err)
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

	if _, err := fv.BluetoothUIService.BluetoothDeviceDetail(ctx, &bts.BluetoothDeviceDetailRequest{
		Name:        device.AdvertisedName(),
		MatchOption: bts.BluetoothDeviceDetailRequest_MATCH_OPTION_NOT_REPORT_BATTERY.Enum(),
	}); err != nil {
		s.Fatal("Failed to verify Bluetooth device detail page reports the battery info correctly: the battery info is still presented: ", err)
	}
}

func findPairedDevice(ctx context.Context, bluetoothUIService bts.BluetoothUIServiceClient, predicate func(device *bts.Device) bool) (*bts.Device, error) {
	var device *bts.Device
	// The OS-Settings may not ready immediately after the connection status of the
	// Bluetooth device changes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := bluetoothUIService.CollectDeviceList(ctx, &emptypb.Empty{})
		if err != nil {
			return err
		}
		devices := resp.GetDevices()
		index := slices.IndexFunc(devices, predicate)
		if index == -1 {
			return errors.New("Bluetooth device not found")
		}
		device = devices[index]

		// The OS-Settings may display the incorrect connection status of the Bluetooth
		// device for a while after the connection status changes.
		if device.GetIsConnected() != true {
			return errors.New("Bluetooth device is not connected")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 3 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to found the expected Bluetooth Device from Bluetooth devices page")
	}

	return device, nil
}
