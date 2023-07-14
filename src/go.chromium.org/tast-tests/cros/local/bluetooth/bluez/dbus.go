// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluez

import (
	"context"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
)

// Dbus path constants.
const (
	DBusBluezService               = "org.bluez"
	bluezAdapterIface              = DBusBluezService + ".Adapter1"
	bluezAgentIface                = DBusBluezService + ".Agent1"
	bluezAgentManagerIface         = DBusBluezService + ".AgentManager1"
	bluezBatteryIface              = DBusBluezService + ".Battery1"
	bluezDeviceIface               = DBusBluezService + ".Device1"
	bluezLEAdvertisingManagerIface = DBusBluezService + ".LEAdvertisingManager1"
	bluezAdminPolicyStatusIface    = DBusBluezService + ".AdminPolicyStatus1"
	bluezDebugIface                = "org.chromium.Bluetooth.Debug"
)

// NewBluezDBusObject creates a new dbusutil.DBusObject with the service
// parameter prefilled as DBusBluezService.
func NewBluezDBusObject(ctx context.Context, objIface string, path dbus.ObjectPath) (*dbusutil.DBusObject, error) {
	return dbusutil.NewDBusObject(ctx, DBusBluezService, objIface, path)
}

func collectExistingBluezObjectPaths(ctx context.Context, objIface string) ([]dbus.ObjectPath, error) {
	return dbusutil.CollectExistingServiceObjectPaths(ctx, DBusBluezService, objIface)
}

func buildNewUniqueObjectPath(basePath string) dbus.ObjectPath {
	if basePath == "" {
		basePath = "/test"
	}
	basePath = strings.TrimSuffix(basePath, "/")
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	path := fmt.Sprintf("%s/%s", basePath, id)
	return dbus.ObjectPath(path)
}
