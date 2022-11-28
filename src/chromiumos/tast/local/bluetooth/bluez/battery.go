// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluez

import (
	"context"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/local/dbusutil"
)

// Battery is the bluez dbus battery object abstraction.
type Battery struct {
	dbus *dbusutil.DBusObject
}

// NewBattery creates a new bluetooth Battery from the passed D-Bus object path.
func NewBattery(ctx context.Context, path dbus.ObjectPath) (*Battery, error) {
	obj, err := NewBluezDBusObject(ctx, bluezBatteryIface, path)
	if err != nil {
		return nil, err
	}
	return &Battery{dbus: obj}, nil
}

// Batteries creates a Battery for all bluetooth batteries in the system.
func Batteries(ctx context.Context) ([]*Battery, error) {
	paths, err := collectExistingBluezObjectPaths(ctx, bluezBatteryIface)
	if err != nil {
		return nil, err
	}
	batteries := make([]*Battery, len(paths))
	for i, path := range paths {
		battery, err := NewBattery(ctx, path)
		if err != nil {
			return nil, err
		}
		batteries[i] = battery
	}
	return batteries, nil
}

// Path gets the D-Bus path this battery was created from.
func (b *Battery) Path() dbus.ObjectPath {
	return b.dbus.ObjectPath()
}

// Percentage returns the percentage of the battery.
func (b *Battery) Percentage(ctx context.Context) (uint8, error) {
	return b.dbus.PropertyUint8(ctx, "Percentage")
}
