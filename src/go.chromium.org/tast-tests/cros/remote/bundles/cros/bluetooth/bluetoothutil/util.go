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
	btr "go.chromium.org/tast-tests/cros/remote/bluetooth"
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

// DiscoverAndPairTimeout is the common timeout for function DiscoverAndPairDevice.
const DiscoverAndPairTimeout = 45 * time.Second

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

// ConfigureAudioDevice configures the peer as an audio device.
func ConfigureAudioDevice(ctx context.Context, device *btr.EmulatedBTPeerDevice, audioProfile cbt.AudioProfile, audioServer cbt.AudioServer, a2dpCodec cbt.A2DPCodec) error {
	audioConfig := &cbt.AudioConfig{
		AudioServer: cbt.AudioServerPulseaudio,
	}
	audioConfig.Update(&cbt.AudioConfig{
		AudioServer: audioServer,
		A2DPCodec:   a2dpCodec,
	})

	if err := device.RPCAudio().SetAudioConfig(ctx, audioConfig); err != nil {
		return errors.Wrap(err, "failed to set audio config")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := device.RPCAudio().StartAudioServer(ctx, audioProfile); err != nil {
			return errors.New("failed to start audio server")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 4 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to start audio server")
	}

	useOfono := false
	for _, p := range cbt.GetOfonoSupportedProfiles() {
		if audioProfile == p {
			useOfono = true
			break
		}
	}
	if useOfono {
		if err := device.RPCAudio().StartOfono(ctx); err != nil {
			return errors.New("start Ofono failed")
		}
	} else {
		if err := device.RPCAudio().StopOfono(ctx); err != nil {
			return errors.New("stop Ofono failed")
		}
	}
	return nil
}
