// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/base64"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// forgetDeviceTestParams includes test specific args.
type forgetDeviceTestParams struct {
	flossEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FastPairForgetDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the Fast Pair forget device scenario",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
			"dclasson@google.com",
		},
		BugComponent: "b:1133283",
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(bluetooth.FastPairHardwareDep),
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.ui.ChromeUIService",
		},
		Timeout: 3 * time.Minute,
		Vars:    []string{bluetooth.TestVarFastPairAntispoofingKeyPem},
		Params: []testing.Param{
			{
				Name: "floss_disabled",
				Val: forgetDeviceTestParams{
					flossEnabled: false,
				},
				Fixture:   "chromeLoggedInAsUserWithFastPairAnd1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name: "floss_enabled",
				Val: forgetDeviceTestParams{
					flossEnabled: true,
				},
				Fixture:           "chromeLoggedInAsUserWithFastPairAnd1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// FastPairForgetDevice tests the Fast Pair forget device scenario.
func FastPairForgetDevice(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	p := s.Param().(forgetDeviceTestParams)

	// Parse antispoofing key pem from test var.
	antispoofingKeyPemBase64 := s.RequiredVar(bluetooth.TestVarFastPairAntispoofingKeyPem)
	antispoofingKeyPem, err := base64.StdEncoding.DecodeString(antispoofingKeyPemBase64)
	if err != nil {
		s.Fatalf("Failed to decode %q from base64 string: %v", bluetooth.TestVarFastPairAntispoofingKeyPem, err)
	}

	crUISvc := ui.NewChromeUIServiceClient(fv.DUTRPCClient.Conn)
	defer func() {
		if !s.HasError() {
			return
		}
		if _, err := crUISvc.DumpUITree(ctx, &emptypb.Empty{}); err != nil {
			testing.ContextLog(ctx, "Failed to dump UI tree: ", err)
		}
	}()

	// Open the Saved Devices subpage and confirm that its empty.
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}

	// Configure btpeer as a fast pair device.
	testing.ContextLog(ctx, "Configuring btpeer as a fast pair device with an antispoofing key pem set")
	fastPairDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeLEFastPair,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer as a fast pair device: ", err)
	}
	if err := fastPairDevice.RPCFastPair().SetAntispoofingKeyPem(ctx, antispoofingKeyPem); err != nil {
		s.Fatal("Failed to set antispoofing key pem on fast pair btpeer: ", err)
	}

	testing.ContextLog(ctx, "Pairing device with fast pair notification")
	if _, err := fv.BluetoothUIService.PairWithFastPairNotification(ctx, &bts.PairWithFastPairNotificationRequest{
		Protocol: bts.FastPairProtocol_FAST_PAIR_PROTOCOL_INITIAL,
	}); err != nil {
		s.Fatal("Failed to pair with fast pair notification: ", err)
	}
	resp, err := fv.BluetoothService.DeviceIsPaired(ctx, &bts.DeviceIsPairedRequest{
		DeviceAddress: fastPairDevice.LocalBluetoothAddress(),
	})
	if err != nil {
		s.Fatal("Failed to check if target device is paired: ", err)
	}
	if !resp.DeviceIsPaired {
		s.Fatal("Fast pair device not paired as expected")
	}

	// Re-open the Saved Devices subpage to refresh the results and confirm the device was added.
	deviceName := fastPairDevice.AdvertisedName()
	if p.flossEnabled {
		// TODO(b/263980939): Floss tests use the advertised name instead of the display name.
		deviceName = "Autotest Test Device"
	}
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{
			deviceName,
		},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}

	// Open the Bluetooth Settings page and forget the device.
	if _, err := fv.BluetoothUIService.ForgetBluetoothDevice(ctx, &bts.ForgetBluetoothDeviceRequest{
		DeviceName: deviceName,
	}); err != nil {
		s.Fatalf("Failed to forget the device with name %s: %v", deviceName, err)
	}

	// Open the Saved Devices subpage and confirm that its empty.
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}
}
