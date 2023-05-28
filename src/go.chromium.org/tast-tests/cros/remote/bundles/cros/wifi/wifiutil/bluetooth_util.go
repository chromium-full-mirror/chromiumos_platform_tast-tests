// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"google.golang.org/grpc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const pollTimeout = 30 * time.Second
const pollInterval = time.Second

// NewChromeServiceClient creates a new ChromeServiceClient and configures
// chrome with the settings needed for bluetooth in wifi tests.
func NewChromeServiceClient(ctx context.Context, dutConn *grpc.ClientConn, chromeSigninProfileTestExtensionID string) (ui.ChromeServiceClient, error) {
	chromeService := ui.NewChromeServiceClient(dutConn)
	if _, err := chromeService.New(ctx, &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		KeepState:                    true,
		SigninProfileTestExtensionId: chromeSigninProfileTestExtensionID,
		DisableFeatures:              []string{"Floss"},
	}); err != nil {
		return nil, errors.Wrap(err, "failed to configure chrome for wifi bluetooth testing on DUT")
	}
	return chromeService, nil
}

// NewBluetoothServiceClient creates a new BluetoothServiceClient and sets
// the stack to always be bluez for use in wifi tests.
func NewBluetoothServiceClient(ctx context.Context, dutConn *grpc.ClientConn) (bluetooth.BluetoothServiceClient, error) {
	bluetoothService := bluetooth.NewBluetoothServiceClient(dutConn)
	if _, err := bluetoothService.SetBluetoothStack(ctx, &bluetooth.SetBluetoothStackRequest{
		StackType: bluetooth.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to set DUT bluetooth stack to bluez")
	}
	return bluetoothService, nil
}

// PollBluetoothBootPref polls the DUT's saved bluetooth preference until the context deadline is exceeded or until a result is returned. If an unexpected result is seen, the function emits an error.
func PollBluetoothBootPref(ctx context.Context, bluetoothService bluetooth.BluetoothServiceClient, adapterEnabledOnBoot bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if response, err := bluetoothService.EnabledOnBoot(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "could not get bluetooth EnabledOnBoot status")
		} else if response.AdapterEnabledOnBoot != adapterEnabledOnBoot {
			return testing.PollBreak(errors.Wrapf(err, "Bluetooth adapterEnabledOnBoot pref is %t, expected to be %t", response.AdapterEnabledOnBoot, adapterEnabledOnBoot))
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  pollTimeout,
		Interval: pollInterval,
	})
}

// PollBluetoothPoweredStatus polls the DUT's bluetooth adapter powered setting until the context deadline is exceeded or until the correct power setting is observed.
func PollBluetoothPoweredStatus(ctx context.Context, bluetoothService bluetooth.BluetoothServiceClient, isPoweredOn bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if response, err := bluetoothService.IsPoweredOn(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to get bluetooth IsPoweredOn status")
		} else if response.IsPoweredOn != isPoweredOn {
			return errors.Errorf("Bluetooth IsPoweredOn is %t, expected %t", response.IsPoweredOn, isPoweredOn)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  pollTimeout,
		Interval: pollInterval,
	})
}

// ValidateBluetoothFunctional validates that bluetooth is function on the DUT
// by toggling bluetooth discovery. We don't actually care about the discovery
// contents, just whether the discovery failed or not. We can stop the scan
// immediately.
func ValidateBluetoothFunctional(ctx context.Context, bluetoothService bluetooth.BluetoothServiceClient) error {
	if _, err := bluetoothService.StartDiscovery(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start bluetooth adapter discovery")
	}
	if _, err := bluetoothService.StopDiscovery(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to stop bluetooth adapter discovery")
	}
	return nil
}

// AssertBluetoothEnabledState checks to see if bluetooth is enabled or disabled
// as expected.
//
// Bluetooth is enabled if the boot preference is to be enabled, the adapter is
// powered on, and the adapter is validated as functional.
//
// Bluetooth is disabled if the boot preference is to be disabled and the adapter
// is powered off.
func AssertBluetoothEnabledState(ctx context.Context, bluetoothService bluetooth.BluetoothServiceClient, bluetoothEnabled bool) (err error) {
	testing.ContextLog(ctx, "Getting BT boot pref")
	if err := PollBluetoothBootPref(ctx, bluetoothService, bluetoothEnabled); err != nil {
		return errors.Wrapf(err, "failed to wait for BT boot pref to be %t", bluetoothEnabled)
	}
	testing.ContextLog(ctx, "Getting BT powered status")
	if err := PollBluetoothPoweredStatus(ctx, bluetoothService, bluetoothEnabled); err != nil {
		return errors.Wrapf(err, "failed to wait for BT powered status to be %t", bluetoothEnabled)
	}
	if bluetoothEnabled {
		testing.ContextLog(ctx, "Validating BT is functional")
		if err := ValidateBluetoothFunctional(ctx, bluetoothService); err != nil {
			return errors.Wrap(err, "failed to validate Bluetooth is functional")
		}
	}
	return err
}
