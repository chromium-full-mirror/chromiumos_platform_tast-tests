// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package modemloggerd interacts with modemloggerd D-Bus service.
package modemloggerd

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/modemloggerdconst"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Manager wraps a Modemloggerd.Manager DBus object.
type Manager struct {
	*dbusutil.DBusObject
}

// getModemloggerdManager returns a modemloggerd manager object that can be used to list available eSIMs.
func getModemloggerdManager(ctx context.Context) (*Manager, error) {
	obj, err := dbusutil.NewDBusObject(ctx, modemloggerdconst.DBusModemloggerdService, modemloggerdconst.DBusModemloggerdManagerInterface, modemloggerdconst.DBusModemloggerdManagerPath)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to Modemloggerd")
	}
	return &Manager{obj}, nil
}

// waitForModemLoggerdManager polls until Modemloggerd exports a Manager object on DBUS
func waitForModemLoggerdManager(ctx context.Context) (*Manager, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := getModemloggerdManager(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to find ModemLogger")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 200 * time.Millisecond,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to find a modemloggerd Manager object")
	}
	return getModemloggerdManager(ctx)
}

// GetModemLoggerPaths waits for modemloggerd to export a manager object and returns paths to DBus objects capable of logging
func GetModemLoggerPaths(ctx context.Context) ([]dbus.ObjectPath, error) {
	h, err := waitForModemLoggerdManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get Modemloggerd Manager DBus object")
	}
	props, err := dbusutil.NewDBusProperties(ctx, h.DBusObject)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get Modemloggerd manager properties")
	}
	modemPaths, err := props.GetObjectPaths(modemloggerdconst.ManagerPropertyAvailableModems)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get available modems")
	}
	return modemPaths, nil
}
