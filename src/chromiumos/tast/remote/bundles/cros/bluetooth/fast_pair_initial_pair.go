// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"encoding/base64"
	"time"

	cbt "chromiumos/tast/common/chameleon/devices/common/bluetooth"
	"chromiumos/tast/remote/bluetooth"
	pb "chromiumos/tast/services/cros/bluetooth"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FastPairInitialPair,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the Fast Pair initial pairing scenario",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
			"dclasson@google.com",
		},
		BugComponent: "b:1133283",
		Attr:         []string{"group:bluetooth", "bluetooth_cross_device_fastpair"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BTTestService"},
		Fixture:      "chromeLoggedInAsUserWithFastPairAnd1BTPeer",
		Timeout:      3 * time.Minute,
		Vars:         []string{bluetooth.TestVarFastPairAntispoofingKeyPem},
	})
}

// FastPairInitialPair tests the Fast Pair initial pairing scenario.
func FastPairInitialPair(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	// Parse antispoofing key pem from test var.
	antispoofingKeyPemBase64 := s.RequiredVar(bluetooth.TestVarFastPairAntispoofingKeyPem)
	antispoofingKeyPem, err := base64.StdEncoding.DecodeString(antispoofingKeyPemBase64)
	if err != nil {
		s.Fatalf("Failed to decode %q from base64 string: %v", bluetooth.TestVarFastPairAntispoofingKeyPem, err)
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

	resp, err := fv.BTS.DeviceStatus(ctx, &pb.DeviceStatusRequest{
		Device: fastPairDevice.BTSDevice(),
	})
	if err != nil {
		s.Fatal("Failed to check if target device is paired: ", err)
	}
	if !resp.IsPaired {
		s.Fatal("Fast pair device not paired as expected")
	}
}
