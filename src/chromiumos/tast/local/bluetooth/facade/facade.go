// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package facade

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/bluetooth/facade/bluez"
	"chromiumos/tast/local/bluetooth/facade/common"
	"chromiumos/tast/local/bluetooth/facade/floss"
	"chromiumos/tast/testing"
)

var bluetoothFacadeSingleton common.BluetoothFacade = nil

// NewBluetoothFacade initializes the configures the DUT to use the specified
// bluetooth stack and initializes the corresponding BluetoothFacade
// implementation for that stack. Only one BluetoothFacade may exist at one
// time, and this method should be the only way that they are initialized.
//
// If the last initialized bluetooth stack is the same as the desired stack,
// the BluetoothFacade instance will not be recreated, but instead reused. Thus,
// it is safe to repeatedly call this function to get the same facade for the
// same stack.
//
// If the last initialized bluetooth stack is differs from the desired stack,
// the previous BluetoothFacade instance will be disabled prior to creating
// a new instance with the desired stack.
//
// Note: Even if you do not need to use the facade, this method is also intended
// to be the way to configure the DUT to use a given stack. So for example, if
// a test or fixture is bluez-dependent, it should always make a call to this
// function with bluez as the stackType so that it can ensure the DUT is
// configured to use bluez.
func NewBluetoothFacade(ctx context.Context, stackType common.BluetoothStackType) (common.BluetoothFacade, error) {
	if bluetoothFacadeSingleton != nil {
		currentStackType := bluetoothFacadeSingleton.StackType()
		if currentStackType == stackType {
			testing.ContextLogf(ctx, "Reusing existing %s bluetooth facade", currentStackType)
			return bluetoothFacadeSingleton, nil
		}
		testing.ContextLogf(ctx, "Disabling existing %s bluetooth facade", currentStackType)
		if err := bluetoothFacadeSingleton.Disable(ctx); err != nil {
			bluetoothFacadeSingleton = nil
			return nil, errors.Wrapf(err, "failed to disable existing %s bluetooth facade", currentStackType)
		}
		bluetoothFacadeSingleton = nil
	}
	var facade common.BluetoothFacade
	switch stackType {
	case common.BluetoothStackTypeBluez:
		if err := floss.SetFlossEnabled(ctx, false); err != nil {
			return nil, errors.Wrap(err, "failed to disable floss prior to enabling bluez")
		}
		testing.ContextLog(ctx, "Initializing new bluez bluetooth facade")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var err error
			facade, err = bluez.NewBluetoothBluezFacade(ctx)
			return err
		}, &testing.PollOptions{
			Interval: 1 * time.Second,
			Timeout:  10 * time.Second,
		}); err != nil {
			return nil, errors.Wrap(err, "failed to initialize new bluez bluetooth facade")
		}
	case common.BluetoothStackTypeFloss:
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
