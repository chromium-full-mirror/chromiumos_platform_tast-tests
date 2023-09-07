// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package missiveutil provides helpers for missive based reporting tests.
package missiveutil

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ResetMissived resets reporting state clearing any existing records and keys.
func ResetMissived(ctx context.Context) error {
	if err := os.RemoveAll("/var/spool/reporting"); err != nil {
		return errors.Wrap(err, "could not remove missive dir")
	}
	if err := upstart.RestartJob(ctx, "missived"); err != nil {
		return errors.Wrap(err, "could not restart missived")
	}
	return nil
}

// GetMissiveDbusAddress returns the dBus address of the Missive process.
func GetMissiveDbusAddress(ctx context.Context) (string, error) {
	dbo, err := dbusutil.NewDBusObject(ctx, "org.freedesktop.DBus", "org.freedesktop.DBus",
		"/org/freedesktop/DBus")
	if err != nil {
		return "", errors.Wrap(err, "unable to get DBus object")
	}

	missiveProcessID, err := getProcessID(ctx, dbo, "org.chromium.Missived")
	if err != nil {
		return "", errors.Wrap(err, "failed to get missive's process ID")
	}

	var missiveDbusAddress string
	// Missive may have just been restarted so Poll for a bit until it
	// establishes a dbus connection.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return getDbusAddress(ctx, dbo, missiveProcessID, &missiveDbusAddress)
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return "", errors.Wrap(err, "timed out waiting for missive to be available")
	}
	return missiveDbusAddress, nil
}

// TODO(b/299655895): move getDbusAddress to somewhere in dbusutil/ directory

// getDbusAddress populates `dbusAddress` with the dBus address corresponding to `agentPid`, or returns an error if the address cannot be determined.
func getDbusAddress(ctx context.Context, dbo *dbusutil.DBusObject, agentPid uint64, dbusAddress *string) error {
	var names []string
	if err := dbo.Call(ctx, "ListNames").Store(&names); err != nil {
		return errors.Wrap(err, "Unable to retrieve a list of dbus connection names")
	}
	for _, name := range names {
		var namePid uint64
		// GetConnectionUnixProcessId to fetch the PID associated with each
		// connection and then find the one that matches `agentPid`.
		if err := dbo.Call(ctx, "GetConnectionUnixProcessID", name).Store(&namePid); err != nil {
			continue
		}
		if namePid == agentPid {
			*dbusAddress = name
			return nil
		}
	}
	return errors.New("Unable to determine the dbus connection name for PID " + strconv.FormatUint(uint64(agentPid), 10))
}

// TODO(b/299655895): move getProcessID tosomewhere in dbusutil/ directory

// getProcessID returns the process ID for `processName`, e.g.  "org.chromium.Missived", or an error if the process ID cannot be determined.
func getProcessID(ctx context.Context, dbo *dbusutil.DBusObject, processName string) (uint64, error) {
	var namePid uint64
	if err := dbo.Call(ctx, "GetConnectionUnixProcessID", processName).Store(&namePid); err != nil {
		return 0, errors.Wrap(err, "Unable to get DBus object")
	}
	return namePid, nil
}
