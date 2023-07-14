// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bluez contains helpers to interact with the system's bluetooth bluez
// adapters.
package bluez

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
)

// Debug is the bluetooth dbus debug interface abstraction.
type Debug struct {
	dbus *dbusutil.DBusObject
}

// NewDebug creates a new dbus object for the debug interface.
func NewDebug(ctx context.Context) (*Debug, error) {
	obj, err := NewBluezDBusObject(ctx, bluezDebugIface, "/org/chromium/Bluetooth")
	if err != nil {
		return nil, err
	}
	return &Debug{dbus: obj}, nil
}

// SetLevels sets the debug level for bluez and kernel.
func (d *Debug) SetLevels(ctx context.Context, bluez, kernel int) {
	d.dbus.Call(ctx, "SetLevels", byte(bluez), byte(kernel))
}
