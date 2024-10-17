// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package floss provides a Floss implementation of the Bluetooth interface.
package floss

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
)

// Floss services.
const (
	DBusFlossService        = "org.chromium.bluetooth"
	DBusFlossManagerService = "org.chromium.bluetooth.Manager"
)

// Floss object interfaces.
const (
	flossManagerInterface                     = DBusFlossManagerService
	flossAdapterInterface                     = DBusFlossService + ".Bluetooth"
	flossBluetoothCallbackInterface           = DBusFlossService + ".BluetoothCallback"
	flossBluetoothConnectionCallbackInterface = DBusFlossService + ".BluetoothConnectionCallback"
)

// Floss object path suffixes for objects that are tied to a specific adapter.
const (
	flossAdapterObjectPathSuffix                     = "adapter"
	flossBluetoothCallbackObjectPathSuffix           = "test_adapter_cb"
	flossBluetoothConnectionCallbackObjectPathSuffix = "test_adapter_connection_cb"
)

// NewFlossDBusObject creates a new dbusutil.DBusObject with the service
// parameter prefilled as DBusFlossService.
func NewFlossDBusObject(ctx context.Context, objIface string, path dbus.ObjectPath) (*dbusutil.DBusObject, error) {
	return dbusutil.NewDBusObject(ctx, DBusFlossService, objIface, path)
}

// NewFlossManagerDBusObject creates a new dbusutil.DBusObject with the service
// parameter prefilled as DBusFlossManagerService.
func NewFlossManagerDBusObject(ctx context.Context, objIface string, path dbus.ObjectPath) (*dbusutil.DBusObject, error) {
	return dbusutil.NewDBusObject(ctx, DBusFlossManagerService, objIface, path)
}

func collectExistingFlossManagerServiceObjectPaths(ctx context.Context, objIface string) ([]dbus.ObjectPath, error) {
	return dbusutil.CollectExistingServiceObjectPaths(ctx, DBusFlossManagerService, objIface)
}

func buildAdapterSpecificObjectPath(adapterHCI int, objectPathSuffix string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("/org/chromium/bluetooth/hci%d/%s", adapterHCI, objectPathSuffix))
}
