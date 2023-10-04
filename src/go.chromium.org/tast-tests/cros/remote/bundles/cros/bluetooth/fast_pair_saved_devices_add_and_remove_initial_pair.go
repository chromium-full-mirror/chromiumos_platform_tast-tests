// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/base64"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// savedDeviceTestParams includes test specific args.
type savedDeviceTestParams struct {
	flossEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FastPairSavedDevicesAddAndRemoveInitialPair,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that Saved Devices subpage shows a newly added Saved Device after the Fast Pair initial pairing scenario, and removes the device after clicking Remove",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
			"dclasson@google.com",
		},
		BugComponent: "b:1133283",
		Attr: []string{
			"group:bluetooth",
			"bluetooth_cross_device_fastpair_multidut",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(bluetooth.FastPairHardwareDep),
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.ui.ChromeUIService",
		},
		Timeout: 3 * time.Minute,
		VarDeps: []string{bluetooth.TestVarFastPairAntispoofingKeyPem},
		Params: []testing.Param{
			{
				Name: "floss_disabled",
				Val: savedDeviceTestParams{
					flossEnabled: false,
				},
				Fixture: "twoChromebooksLoggedInWithFastPairAnd1BTPeerFlossDisabled",
			},
			{
				Name: "floss_enabled",
				Val: savedDeviceTestParams{
					flossEnabled: true,
				},
				Fixture:           "twoChromebooksLoggedInWithFastPairAnd1BTPeerFlossEnabled",
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// FastPairSavedDevicesAddAndRemoveInitialPair tests that devices are saved to
// the "Saved Devices" page during Initial Pair, and this propagates to
// companion devices. It also tests that "removing" from the "Saved Devices"
// page is propagated to companion devices.
func FastPairSavedDevicesAddAndRemoveInitialPair(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	p := s.Param().(savedDeviceTestParams)

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
			s.Log(ctx, "Failed to dump UI tree: ", err)
		}
	}()

	// Open the Saved Devices subpage on both DUTs and confirm that it's empty.
	if err := confirmSavedDevicesStateBothDUTs(ctx, fv, []string{} /*deviceNames*/); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}

	// Configure btpeer as a fast pair device.
	s.Log(ctx, "Configuring btpeer as a fast pair device with an antispoofing key pem set")
	fastPairDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeLEFastPair,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer as a fast pair device: ", err)
	}
	if err := fastPairDevice.RPCFastPair().SetAntispoofingKeyPem(ctx, antispoofingKeyPem); err != nil {
		s.Fatal("Failed to set antispoofing key pem on fast pair btpeer: ", err)
	}

	// Pair with the device on the primary DUT only.
	s.Log(ctx, "Pairing device with fast pair notification on primary DUT")
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

	// Re-open the Saved Devices subpage on both DUTs to refresh the results and confirm the device was added.
	deviceName := fastPairDevice.AdvertisedName()
	if p.flossEnabled {
		// TODO(b/263980939): Floss tests use the advertised name instead of the display name.
		deviceName = "Autotest Test Device"
	}
	if err := confirmSavedDevicesStateBothDUTs(ctx, fv, []string{deviceName} /*deviceNames*/); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}

	// Remove the saved device from the companion DUT only.
	s.Log(ctx, "Removing device from the Saved Devices page on companion DUT")
	if _, err := fv.CompanionDUTConfig(1).BluetoothUIService.RemoveAllSavedDevices(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to remove the saved device from the companion DUT: ", err)
	}

	// Re-open the Saved Devices subpage on both DUTs and confirm that it's empty.
	if err := confirmSavedDevicesStateBothDUTs(ctx, fv, []string{} /*deviceNames*/); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}
}

func confirmSavedDevicesStateBothDUTs(ctx context.Context, fv *bluetooth.FixtValue, deviceNames []string) error {
	var request = &bts.ConfirmSavedDevicesStateRequest{
		DeviceNames: deviceNames,
	}

	// Open the Saved Devices subpage on the primary DUT and confirm the state.
	if _, err := fv.BluetoothUIService.ConfirmSavedDevicesState(ctx, request); err != nil {
		return errors.Wrap(err, "failed to confirm the state of the Saved Devices subpage on primary DUT")
	}

	// Open the Saved Devices subpage on the companion DUT and confirm the state.
	if _, err := fv.CompanionDUTConfig(1).BluetoothUIService.ConfirmSavedDevicesState(ctx, request); err != nil {
		return errors.Wrap(err, "failed to confirm the state of the Saved Devices subpage on companion DUT")
	}

	return nil
}
