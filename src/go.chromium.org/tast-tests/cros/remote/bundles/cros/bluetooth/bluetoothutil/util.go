// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bluetoothutil provides common functions used by bluetooth test.
package bluetoothutil

import (
	"context"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/servo"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/durationpb"
)

// DeviceTypeTestParam is a type that can be used a test param type when the
// only param is the device type. Prevents the need to make individual test
// param types as many just need the device type.
type DeviceTypeTestParam struct {
	DeviceType cbt.DeviceType
}

// TurnOffServoKeyboardIfOn turns off servo keyboard if on.
func TurnOffServoKeyboardIfOn(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	pxy, err := servo.NewProxy(ctx, s.RequiredVar("servo"), dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)
	if err := pxy.Servo().SetOnOff(ctx, servo.USBKeyboard, servo.Off); err != nil {
		s.Fatal("Failed to turn of servo: ", err)
	}
}

// DiscoverAndPairDevice will use the provided bluetooth service to turn on
// discovery, wait until the device is discovered, turn discovery back off,
// then pair the device. A nil return means that the device has been
// successfully discovered, paired, and connected to the service (connection
// occurs during pairing process).
func DiscoverAndPairDevice(ctx context.Context, bluetoothService bts.BluetoothServiceClient, deviceAddress, devicePin string, discoveryTimeout time.Duration) error {
	if _, err := bluetoothService.DiscoverDevice(ctx, &bts.DiscoverDeviceRequest{
		DeviceAddress:    deviceAddress,
		DiscoveryTimeout: durationpb.New(discoveryTimeout),
	}); err != nil {
		return errors.Wrapf(err, "failed to discover device with address %q", deviceAddress)
	}
	if _, err := bluetoothService.PairDevice(ctx, &bts.PairDeviceRequest{
		DeviceAddress: deviceAddress,
		Pin:           devicePin,
	}); err != nil {
		return errors.Wrapf(err, "failed to pair device with address %q after successful discovery", deviceAddress)
	}
	return nil
}
