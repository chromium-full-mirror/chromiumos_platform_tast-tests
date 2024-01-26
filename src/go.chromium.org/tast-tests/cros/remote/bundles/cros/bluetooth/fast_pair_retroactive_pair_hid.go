// Copyright 2024 The ChromiumOS Authors
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
		Func:         FastPairRetroactivePairHID,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the Fast Pair retroactive pairing scenario with a HID. Floss only test",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
			"laikatherine@google.com",
		},
		BugComponent: "b:1133283",
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair", "bluetooth_floss_flaky"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome", "bluetooth_floss"},
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

// FastPairRetroactivePairHID tests the Fast Pair retroactive pairing scenario with a HID.
func FastPairRetroactivePairHID(ctx context.Context, s *testing.State) {
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

	// Configure btpeer as a fast pair device.
	s.Log("Configuring btpeer as a fast pair device with an antispoofing key pem set and 'Just Works' pairing capability")
	fastPairDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeLEFastPair,
		// Since we pair through quick settings, use "Just Works" pairing
		PairingAgentCapability: cbt.PairingAgentCapabilityNoInputNoOutput,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer as a fast pair device: ", err)
	}
	if err := fastPairDevice.RPCFastPair().SetAntispoofingKeyPem(ctx, antispoofingKeyPem); err != nil {
		s.Fatal("Failed to set antispoofing key pem on fast pair btpeer: ", err)
	}
	if err := fastPairDevice.RPCFastPair().SetAdvertiseNameAndHID(ctx, true); err != nil {
		s.Fatal("Failed to configure advertisements on fast pair btpeer: ", err)
	}

	// Pair device through quick settings.
	s.Log("Pairing device with quick settings")
	if _, err := fv.BluetoothUIService.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: fastPairDevice.AdvertisedName(),
	}); err != nil {
		s.Fatalf("Failed to pair the Bluetooth device %q with quick settings: %v", fastPairDevice.String(), err)
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

	// Check we get fast pair retroactive notification and save device to account
	s.Log("Saving device with fast pair notification")
	if _, err := fv.BluetoothUIService.PairWithFastPairNotification(ctx, &bts.PairWithFastPairNotificationRequest{
		Protocol: bts.FastPairProtocol_FAST_PAIR_PROTOCOL_RETROACTIVE,
	}); err != nil {
		s.Fatal("Failed to save with fast pair notification: ", err)
	}

	// Re-open the Saved Devices subpage to refresh the results and confirm the device was added.
	deviceName := fastPairDevice.AdvertisedName()
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{deviceName},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}
}
