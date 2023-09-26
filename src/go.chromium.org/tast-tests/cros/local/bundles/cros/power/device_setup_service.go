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

	opt.KeyboardBrightness = setup.SetKbBrightnessToZero

	switch req.UiAndBacklight {
	case power.UIAndBacklightMode_DO_NOT_CHANGE_UI_WITH_DEFAULT_BACKLIGHT:
		opt.UI = setup.DoNotChangeUI
		opt.Backlight = setup.SetBacklight
	case power.UIAndBacklightMode_DISABLE_UI_WITH_ZERO_BACKLIGHT:
		opt.UI = setup.DisableUI
		opt.Backlight = setup.SetBacklightToZero
	}

	switch req.Wifi {
	case power.WifiInterfacesMode_DO_NOT_CHANGE_WIFI_INTERFACES:
		opt.Wifi = setup.DoNotChangeWifiInterfaces
	case power.WifiInterfacesMode_DISABLE_WIFI_INTERFACES:
		opt.Wifi = setup.DisableWifiInterfaces
	}

	switch req.Bluetooth {
	case power.BluetoothMode_DISABLE_BLUETOOTH_INTERFACES:
		opt.Bluetooth = setup.DisableBluetoothInterfaces
	case power.BluetoothMode_DO_NOT_CHANGE_BLUETOOTH:
		opt.Bluetooth = setup.DoNotChangeBluetooth
	}

	cleanup, err := setup.PowerTestSetup(ctx, setupName, nil, opt)
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
