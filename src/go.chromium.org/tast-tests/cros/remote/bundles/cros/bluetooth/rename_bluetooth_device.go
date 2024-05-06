// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RenameBluetoothDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the connected/disconnected Bluetooth device can be renamed and the edited name is persisted after re-login",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:bluetooth"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps:   []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.browser.ChromeService",
		},
		Params: []testing.Param{
			{
				Name:      "floss_disabled__device_connected",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       true, // expectDeviceIsConnected
			}, {
				Name:      "floss_disabled__device_disconnected",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       false, // expectDeviceIsConnected
			}, {
				Name:              "floss_enabled__device_connected",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               true, // expectDeviceIsConnected
			}, {
				Name:              "floss_enabled__device_disconnected",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               false, // expectDeviceIsConnected
			},
		},
	})
}

// RenameBluetoothDevice verify that the connected/disconnected Bluetooth device can be renamed and the edited name is persisted after re-login.
func RenameBluetoothDevice(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		// In this test the choice of setting the mouse as the Bluetooth device is arbitrary.
		DeviceType: cbt.DeviceTypeMouse,
	})
	if err != nil {
		s.Fatal("Failed to create a new EmulatedBTPeerDevice for the disconnected test: ", err)
	}

	if _, err := fv.BluetoothUIService.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: device.AdvertisedName(),
	}); err != nil {
		s.Fatalf("Failed to pair the Bluetooth device %q with quick settings: %v", device.String(), err)
	}

	expectDeviceIsConnected := s.Param().(bool)
	// Setting up the Bluetooth device to be connected or disconnected by powering its adapter
	// since ChromeOS does not have an interface to do such control by design.
	toggleConnectState := device.RPC().AdapterPowerOn
	if !expectDeviceIsConnected {
		toggleConnectState = device.RPC().AdapterPowerOff
	}
	if err := toggleConnectState(ctx); err != nil {
		s.Fatal("Failed to set the connect state of the Bluetooth device: ", err)
	}

	option := bts.BluetoothDeviceDetailRequest_MATCH_OPTION_CONNECTED.Enum()
	if !expectDeviceIsConnected {
		option = bts.BluetoothDeviceDetailRequest_MATCH_OPTION_DISCONNECTED.Enum()
	}
	// Waiting for the device to be connected/disconnected before proceed on other steps.
	if _, err := fv.BluetoothUIService.BluetoothDeviceDetail(ctx, &bts.BluetoothDeviceDetailRequest{
		Name:        device.AdvertisedName(),
		MatchOption: option,
	}); err != nil {
		s.Fatal("Failed to wait for the Bluetooth device: ", err)
	}

	const deviceNickName = "TestNickName"

	if _, err := fv.BluetoothUIService.RenameBluetoothDevice(ctx, &bts.RenameBluetoothDeviceRequest{
		Device:     &bts.Device{Name: device.AdvertisedName()},
		CustomName: deviceNickName,
	}); err != nil {
		s.Fatalf("Failed to rename the Bluetooth device %q: %v", device.String(), err)
	}
	// TODO(b/292732650): There is no way to reset device's name.

	// Log out and then log back in to further verify if the custom name of the Bluetooth device persists.
	if err := fv.PrimaryDUTConfig().DUTReLogin(ctx); err != nil {
		s.Fatal("Failed to re-login Chrome: ", err)
	}

	// Waiting for the expected Bluetooth device to appear in the OS Settings,
	// as Bluetooth device may not immediately appear after re-login.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Verify that the custom name of the Bluetooth device is persist after re-login.
		resp, err := fv.BluetoothUIService.CollectDeviceList(ctx, &emptypb.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to get the collect of Bluetooth devices")
		}
		if found := slices.IndexFunc(resp.Devices, func(device *bts.Device) bool { return device.Name == deviceNickName }) >= 0; !found {
			return errors.Errorf("failed to find Bluetooth device %q", deviceNickName)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		s.Fatal("Failed to verify the Bluetooth device nickname persist: ", err)
	}
}
