// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bluetoothutil provides common functions used by bluetooth test.
package bluetoothutil

import (
	"context"
	"fmt"
	"path"
	"time"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/servo"
	btr "go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
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

// copyDataFileToHost copies the remote tast data file to the host and returns
// the path to the copied file on the host.
//
// The file transfer is done with linuxssh.PutFiles, which only transfers files
// if they do not already exist and have matching SHA1 checksums.
//
// Note: The hostname parameter is only used for logging purposes and can be any
// descriptive string name for the host.
func copyDataFileToHost(ctx context.Context, sshConn *ssh.Conn, hostname string, dataPaths map[string]string, dataFile string) (string, error) {
	testing.ContextLogf(ctx, "Copying data file %q from remote tast runner to %s", dataFile, hostname)
	const remoteHostDataDir = "/var/tmp/tast/data/src/go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/data"
	remoteDataFilePath := path.Join(remoteHostDataDir, dataFile)
	localDataFilePath, ok := dataPaths[dataFile]
	if !ok {
		return "", errors.Errorf("invalid data file %q: no entry found to local path in dataFiles", dataFile)
	}
	if _, err := linuxssh.PutFiles(ctx, sshConn, map[string]string{
		localDataFilePath: remoteDataFilePath,
	}, linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to copy data file %q from remote tast runner at %q to %s at %q", dataFile, localDataFilePath, remoteDataFilePath, hostname)
	}
	testing.ContextLogf(ctx, "Data file %q available on %s at %q", dataFile, hostname, remoteDataFilePath)
	return remoteDataFilePath, nil
}

// CopyDataFileToDut copies the remote tast data file to the DUT and returns the
// path to the copied file on the DUT.
func CopyDataFileToDut(ctx context.Context, dutConfig *btr.DUTConfig, dataPaths map[string]string, dataFile string) (string, error) {
	return copyDataFileToHost(ctx, dutConfig.DUT.Conn(), fmt.Sprintf("DUT %q", dutConfig.DUT.HostName()), dataPaths, dataFile)
}

// CopyDataFileToBtpeer copies the remote tast data file to the btpeer and
// returns the path to the copied file on the btpeer.
func CopyDataFileToBtpeer(ctx context.Context, btpeerClient *btr.BtpeerClient, dataPaths map[string]string, dataFile string) (string, error) {
	sshConn, err := btpeerClient.SSHConn()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get active ssh connection to %s", btpeerClient)
	}
	return copyDataFileToHost(ctx, sshConn, btpeerClient.String(), dataPaths, dataFile)
}
