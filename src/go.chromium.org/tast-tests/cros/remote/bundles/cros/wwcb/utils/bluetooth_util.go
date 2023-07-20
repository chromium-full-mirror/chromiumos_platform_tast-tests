// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DiscoverBluetoothDevice is for discover bluetooth device.
func DiscoverBluetoothDevice(ctx context.Context, deviceName string, bt bluetooth.BluetoothServiceClient) (string, error) {

	if _, err := bt.SetBluetoothStack(ctx, &bluetooth.SetBluetoothStackRequest{
		StackType: 1,
	}); err != nil {
		return "", errors.Wrap(err, "failed to set bluetooth stack")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if _, err := bt.StartDiscovery(ctx, &empty.Empty{}); err != nil {
		return "", errors.Wrap(err, "failed to start bluetooth discovery")
	}

	defer func() {
		if _, err := bt.StopDiscovery(cleanupCtx, &empty.Empty{}); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to stop discovery: ", err)
		}
	}()

	var targetAddress string
	err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := bt.Devices(ctx, &empty.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to find devices")
		}
		for _, address := range devices.KnownDeviceAddresses {
			devName, err := bt.DeviceName(ctx, &bluetooth.DeviceNameRequest{
				DeviceAddress: address,
			})
			if err == nil && devName.DeviceName == deviceName {
				targetAddress = address
				return nil
			}
		}
		return errors.Errorf("failed to find device %s", deviceName)
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})

	if err != nil {
		return "", errors.Wrap(err, "device not found")
	}

	return targetAddress, nil
}

// RemoveBluetoothDevice is for remove bluetooth device.
func RemoveBluetoothDevice(ctx context.Context, address string, bt bluetooth.BluetoothServiceClient) error {
	if _, err := bt.DisconnectDevice(ctx, &bluetooth.DisconnectDeviceRequest{
		DeviceAddress: address,
	}); err != nil {
		return errors.Wrap(err, "failed to disconnect device")
	}

	if _, err := bt.RemoveDevice(ctx, &bluetooth.RemoveDeviceRequest{
		DeviceAddress: address,
	}); err != nil {
		return errors.Wrap(err, "failed to remove device")
	}

	return nil
}
