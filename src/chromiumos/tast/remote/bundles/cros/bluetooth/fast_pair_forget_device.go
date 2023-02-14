// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/base64"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "chromiumos/tast/common/chameleon/devices/common/bluetooth"
	"chromiumos/tast/remote/bluetooth"
	pb "chromiumos/tast/services/cros/bluetooth"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

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
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair", "bluetooth_flaky"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(bluetooth.FastPairHardwareDep),
		ServiceDeps:  []string{"tast.cros.bluetooth.BTTestService"},
		Fixture:      "chromeLoggedInAsUserWithFastPairAnd1BTPeer",
		Timeout:      3 * time.Minute,
		Vars:         []string{bluetooth.TestVarFastPairAntispoofingKeyPem},
	})
}

// FastPairForgetDevice tests the Fast Pair forget device scenario.
func FastPairForgetDevice(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

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
	if _, err := fv.BTS.ConfirmSavedDevicesState(ctx, &pb.ConfirmSavedDevicesStateRequest{
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
	if _, err := fv.BTS.PairWithFastPairNotification(ctx, &pb.PairWithFastPairNotificationRequest{
		Protocol: pb.FastPairProtocol_FAST_PAIR_PROTOCOL_INITIAL,
	}); err != nil {
		s.Fatal("Failed to pair with fast pair notification: ", err)
	}

	if resp, err := fv.BTS.DeviceStatus(ctx, &pb.DeviceStatusRequest{
		Device: fastPairDevice.BTSDevice(),
	}); err != nil {
		s.Fatal("Failed to check if target device is paired: ", err)
	} else if !resp.IsPaired {
		s.Fatal("Fast pair device not paired as expected")
	}

	// Re-open the Saved Devices subpage to refresh the results and confirm the device was added.
	if _, err := fv.BTS.ConfirmSavedDevicesState(ctx, &pb.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{
			"Autotest Test Device",
		},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}

	// Open the Bluetooth Settings page and forget the device.
	// TODO(b/263980939): It's unexpected that the device name for the fake device is
	// "FAST_PAIR_REF". Investigate why the Raspberry Pi fake device has this name and
	// pass in the same device as above here.
	var deviceName = fastPairDevice.AdvertisedName()
	if _, err := fv.BTS.ForgetBluetoothDevice(ctx, &pb.ForgetBluetoothDeviceRequest{
		DeviceName: deviceName,
	}); err != nil {
		s.Fatalf("Failed to forget the device with name %s: %v", deviceName, err)
	}

	// Open the Saved Devices subpage and confirm that its empty.
	if _, err := fv.BTS.ConfirmSavedDevicesState(ctx, &pb.ConfirmSavedDevicesStateRequest{
		DeviceNames: []string{},
	}); err != nil {
		s.Fatal("Failed to confirm the state of the Saved Devices subpage: ", err)
	}
}
