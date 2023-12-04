// Copyright 2022 The ChromiumOS Authors
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
		Func:         FastPairSubsequentPair,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the Fast Pair subsequent pairing scenario",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
			"dclasson@google.com",
		},
		BugComponent: "b:1133283",
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair_multidut"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(bluetooth.FastPairHardwareDep),
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
		},
		Timeout: 3 * time.Minute,
		Vars: []string{
			bluetooth.TestVarFastPairAntispoofingKeyPem,
		},
		Params: []testing.Param{
			{
				Name:    "floss_disabled",
				Fixture: "twoChromebooksLoggedInWithFastPairAnd1BTPeerFlossDisabled",
			},
			{
				Name:              "floss_enabled",
				Fixture:           "twoChromebooksLoggedInWithFastPairAnd1BTPeerFlossEnabled",
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// FastPairSubsequentPair tests the Fast Pair subsequent pairing scenario.
func FastPairSubsequentPair(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	companionDUTConfig := fv.CompanionDUTConfig(1)

	// Parse antispoofing key pem from test var.
	antispoofingKeyPemBase64 := s.RequiredVar(bluetooth.TestVarFastPairAntispoofingKeyPem)
	antispoofingKeyPem, err := base64.StdEncoding.DecodeString(antispoofingKeyPemBase64)
	if err != nil {
		s.Fatalf("Failed to decode %q from base64 string: %v", bluetooth.TestVarFastPairAntispoofingKeyPem, err)
	}

	// Start with the Companion's Bluetooth off.
	// TODO(b/263407987): Fix the Subsequent Pair scenario when the user has previously observed
	// the not discoverable advertisement, but account key wasn't yet saved to Footprints. We
	// won't be notified of the advertisment again (since it hasn't changed). To workaround
	// this we prevent the Companion from receiving an advertisement until after the primary
	// DUT has finished pairing successfully.
	testing.ContextLog(ctx, "Powering off bluetooth on the Companion DUT")
	if _, err := companionDUTConfig.BluetoothService.SetPowered(ctx, &bts.SetPoweredRequest{
		Powered: false,
	}); err != nil {
		s.Fatal("Failed to power off bluetooth adapter on the companion DUT: ", err)
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

	// Perform Initial pairing on DUT.
	testing.ContextLog(ctx, "Pairing device with Initial fast pair notification on DUT")
	if _, err := fv.BluetoothUIService.PairWithFastPairNotification(ctx, &bts.PairWithFastPairNotificationRequest{
		Protocol: bts.FastPairProtocol_FAST_PAIR_PROTOCOL_INITIAL,
	}); err != nil {
		s.Fatal("Failed to pair with initial fast pair notification: ", err)
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

	// We need to wait for the Primary DUT to complete pairing, which includes a network
	// call to Footprints. There aren't any UI indicators that the network call was
	// finished so we have to use Sleep.
	//GoBigSleepLint TODO(b/263407987): Remove this sleep once the bug is fixed.
	testing.Sleep(ctx, 5*time.Second)

	// Enable Bluetooth on the Companion DUT.
	testing.ContextLog(ctx, "Powering on bluetooth on the Companion DUT")
	if _, err := companionDUTConfig.BluetoothService.SetPowered(ctx, &bts.SetPoweredRequest{
		Powered: true,
	}); err != nil {
		s.Fatal("Failed to power on bluetooth adapter on the companion DUT: ", err)
	}

	// Perform Subsequent pairing on Companion DUT.
	testing.ContextLog(ctx, "Pairing device with Subsequent fast pair notification on DUT")
	if _, err := companionDUTConfig.BluetoothUIService.PairWithFastPairNotification(ctx, &bts.PairWithFastPairNotificationRequest{
		Protocol: bts.FastPairProtocol_FAST_PAIR_PROTOCOL_SUBSEQUENT,
	}); err != nil {
		s.Fatal("Failed to pair with subsequent fast pair notification: ", err)
	}
	resp, err = companionDUTConfig.BluetoothService.DeviceIsPaired(ctx, &bts.DeviceIsPairedRequest{
		DeviceAddress: fastPairDevice.LocalBluetoothAddress(),
	})
	if err != nil {
		s.Fatal("Failed to check if target device is paired: ", err)
	}
	if !resp.DeviceIsPaired {
		s.Fatal("Fast pair device not paired to companion DUT as expected")
	}
}
