// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

const defaultSetupName = "remotePowerTestSetup"

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			power.RegisterDeviceSetupServiceServer(srv, &DeviceSetupService{s: s})
		},
	})
}

// DeviceSetupService implements device setup service.
type DeviceSetupService struct {
	s       *testing.ServiceState
	cleanup setup.CleanupCallback
}

// Setup DUT for power test. The keyboard brightness is set to zero by default.
func (d *DeviceSetupService) Setup(ctx context.Context, req *power.DeviceSetupRequest) (*empty.Empty, error) {
	if d.cleanup != nil {
		testing.ContextLog(ctx, "Found an existing device setup before setting up, attempting to clean-up now")
		if err := d.cleanup(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to clean-up existing device setup before setting up")
		}
		d.cleanup = nil
		testing.ContextLog(ctx, "Clean-up succeeded, continue setting up")
	}

	setupName := defaultSetupName
	if req.SetupName != nil {
		setupName = req.GetSetupName()
	}

	opt := new(setup.PowerTestOptions)

	switch req.Ui {
	case power.UIMode_DO_NOT_CHANGE_UI:
		opt.UI = setup.DoNotChangeUI
	case power.UIMode_DISABLE_UI:
		opt.UI = setup.DisableUI
	}

	switch req.ScreenBrightness {
	case power.ScreenMode_DEFAULT_SCREEN_BRIGHTNESS:
		opt.Backlight = setup.SetBacklight
	case power.ScreenMode_DO_NOT_CHANGE_SCREEN_BRIGHTNESS:
		opt.Backlight = setup.DoNotChangeBacklight
	case power.ScreenMode_ZERO_SCREEN_BRIGHTNESS:
		opt.Backlight = setup.SetBacklightToZero
	}

	switch req.KeyboardBrightness {
	case power.KeyboardMode_DEFAULT_KEYBOARD_BRIGHTNESS:
		opt.KeyboardBrightness = setup.SetKbBrightness
	case power.KeyboardMode_DO_NOT_CHANGE_KEYBOARD_BRIGHTNESS:
		opt.KeyboardBrightness = setup.DoNotChangeKbBrightness
	case power.KeyboardMode_ZERO_KEYBOARD_BRIGHTNESS:
		opt.KeyboardBrightness = setup.SetKbBrightnessToZero
	}

	switch req.Wifi {
	case power.WifiMode_DO_NOT_CHANGE_WIFI:
		opt.Wifi = setup.DoNotChangeWifiInterfaces
	case power.WifiMode_DISABLE_WIFI:
		opt.Wifi = setup.DisableWifiInterfaces
	}

	switch req.Bluetooth {
	case power.BluetoothMode_DISABLE_BLUETOOTH:
		opt.Bluetooth = setup.DisableBluetoothInterfaces
	case power.BluetoothMode_DO_NOT_CHANGE_BLUETOOTH:
		opt.Bluetooth = setup.DoNotChangeBluetooth
	}

	cleanup, _, err := setup.PowerTestSetup(ctx, setupName, nil, opt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup power test")
	}
	d.cleanup = cleanup

	return &empty.Empty{}, nil
}

// Cleanup resets the DUT to its original state.
func (d *DeviceSetupService) Cleanup(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if d.cleanup == nil {
		return nil, errors.New("nothing to clean up as there is no existing device setup")
	}

	if err := d.cleanup(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to clean up existing device setup")
	}

	d.cleanup = nil
	return &empty.Empty{}, nil
}
