// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package facade

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/floss"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var bluetoothFacadeSingleton common.BluetoothFacade = nil

// NewBluetoothFacade initializes the configures the DUT to use the specified
// bluetooth stack and initializes the corresponding BluetoothFacade
// implementation for that stack. Only one BluetoothFacade may exist at one
// time, and this method should be the only way that they are initialized.
//
// Currently Floss stack is initialized regardless of the stack specified,
// as bluez is deprecated
//
// If the last initialized bluetooth stack is the same as the desired stack,
// the BluetoothFacade instance will not be recreated, but instead reused. Thus,
// it is safe to repeatedly call this function to get the same facade for the
// same stack.
//
// If the last initialized bluetooth stack is differs from the desired stack,
// the bluetooth stack is switched and a new facade instance is created.
//
// Note: Even if you do not need to use the facade, this method is also intended
// to be the way to configure the DUT to use a given stack. So for example, if
// a test or fixture is bluez-dependent, it should always make a call to this
// function with bluez as the stackType so that it can ensure the DUT is
// configured to use bluez.
func NewBluetoothFacade(ctx context.Context, stackType common.BluetoothStackType) (common.BluetoothFacade, error) {
	if stackType == common.BluetoothStackTypeBluez {
		testing.ContextLog(ctx, "bluez is deprecated. Floss will be initialized")
		stackType = common.BluetoothStackTypeFloss
	}

	if bluetoothFacadeSingleton != nil {
		currentStackType := bluetoothFacadeSingleton.StackType()
		if currentStackType == stackType {
			testing.ContextLogf(ctx, "Reusing existing %s bluetooth facade", currentStackType)
			return bluetoothFacadeSingleton, nil
		}
		bluetoothFacadeSingleton = nil
	}
	var facade common.BluetoothFacade

	switch stackType {
	case common.BluetoothStackTypeFloss:
		if err := floss.SetFlossEnabled(ctx, true); err != nil {
			return nil, errors.Wrap(err, "failed to enable floss prior to initializing floss facade")
		}
		testing.ContextLog(ctx, "Initializing new floss bluetooth facade")
		var err error
		facade, err = floss.NewBluetoothFlossFacade(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to initialize new floss bluetooth facade")
		}
	default:
		return nil, errors.Errorf("invalid BluetoothStackType %q", stackType)
	}
	bluetoothFacadeSingleton = facade
	testing.ContextLogf(ctx, "Successfully initialized new %s bluetooth facade", bluetoothFacadeSingleton.StackType())
	return bluetoothFacadeSingleton, nil
}

// GetBluetoothStackType determines the active Bluetooth stack on the DUT.
// Returns BluetoothStackTypeUnknown if there's an error determining the stack.
func GetBluetoothStackType(ctx context.Context) (common.BluetoothStackType, error) {
	isFlossEnabled, err := floss.GetFlossEnabled(ctx)
	if err != nil {
		// If the floss manager cannot be found, it's equivalent to floss being disabled.
		if !strings.Contains(err.Error(), `failed to connect to service "org.chromium.bluetooth.Manager"`) {
			return common.BluetoothStackTypeBluez, nil // Floss is implicitly disabled
		}

		// Handle other unexpected errors
		return common.BluetoothStackTypeUnknown, errors.Wrap(err, "failed to get Floss enabled state")
	}

	if isFlossEnabled {
		return common.BluetoothStackTypeFloss, nil
	}

	return common.BluetoothStackTypeBluez, nil
}
