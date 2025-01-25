// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/base64"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FastPairInitialPairKeyboard,
		Desc: "Tests the Fast Pair initial pairing scenario with a keyboard. Floss only test",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"laikatherine@google.com",
			"joaquinmarquez@google.com",
		},
		BugComponent: "b:1133283", // ChromeOS > Software > System Services > Cross Device > Fast Pair
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair", "bluetooth_floss_flaky"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(bluetooth.FastPairHardwareDep),
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
		},
		Timeout: 3 * time.Minute,
		Vars:    []string{bluetooth.TestVarFastPairAntispoofingKeyPem},
		Fixture: "chromeLoggedInAsUserWithFastPairAnd1BTPeerFlossEnabled",
	})
}

// FastPairInitialPairKeyboard tests the Fast Pair initial pairing scenario with a keyboard.
func FastPairInitialPairKeyboard(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	// Parse antispoofing key pem from test var.
	antispoofingKeyPemBase64 := s.RequiredVar(bluetooth.TestVarFastPairAntispoofingKeyPem)
	antispoofingKeyPem, err := base64.StdEncoding.DecodeString(antispoofingKeyPemBase64)
	if err != nil {
		s.Fatalf("Failed to base64 decode key pem %q: %v", bluetooth.TestVarFastPairAntispoofingKeyPem, err)
	}

	// Open the Saved Devices subpage and confirm that its empty.
	s.Log("Confirming Saved Devices subpage is empty")
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}

	// Configure btpeer as a fast pair keyboard device.
	s.Log("Configuring btpeer as a fast pair device with an antispoofing key pem set and 'keyboard only' pairing capability")
	fastPairDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeLEFastPair,
		// Use "keyboard only" pairing to pair with passkey
		PairingAgentCapability: cbt.PairingAgentCapabilityKeyboardOnly,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer as a fast pair device: ", err)
	}
	if err := fastPairDevice.RPCFastPair().SetAntispoofingKeyPem(ctx, antispoofingKeyPem); err != nil {
		s.Fatal("Failed to set antispoofing key pem on fast pair btpeer: ", err)
	}

	// Monitor passkey and send to btpeer
	go func() {
		resp, err := fv.BluetoothService.GetPasskey(ctx, &bts.GetPasskeyRequest{
			DeviceAddress: fastPairDevice.LocalBluetoothAddress(),
		})
		if err != nil {
			s.Fatal("Failed to get passkey: ", err)
		}
		if err := fastPairDevice.RPCFastPair().SetPasskey(ctx, int(resp.Passkey)); err != nil {
			s.Fatal("Failed to set passkey on fast pair btpeer: ", err)
		}
	}()

	s.Log("Pairing device with fast pair notification")
	if _, err := fv.BluetoothUIService.PairWithFastPairNotification(ctx, &bts.PairWithFastPairNotificationRequest{
		Protocol:   bts.FastPairProtocol_FAST_PAIR_PROTOCOL_INITIAL,
		IsKeyboard: true,
	}); err != nil {
		s.Fatal("Failed to pair with fast pair notification: ", err)
	}

	// Check device is paired properly
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
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{deviceName},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}
}
