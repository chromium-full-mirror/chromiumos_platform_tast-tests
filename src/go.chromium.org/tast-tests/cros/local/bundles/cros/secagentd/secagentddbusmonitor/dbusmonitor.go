// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentddbusmonitor is a wrapper around tast/local/dbusutil and
// contains helpers that are used across multiple secagentd tests.
package secagentddbusmonitor

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func getSecagentdDbusConn(ctx context.Context, dbo *dbusutil.DBusObject, agentPid uint64, dbusConn *string) error {
	var names []string
	if err := dbo.Call(ctx, "ListNames").Store(&names); err != nil {
		return errors.Wrap(err, "unable to retrieve a list of dbus connection names")
	}
	for _, name := range names {
		var namePid uint64
		// GetConnectionUnixProcessId to fetch the PID associated with each
		// connection and then find the one that matches secagentd.
		if err := dbo.Call(ctx, "GetConnectionUnixProcessID", name).Store(&namePid); err != nil {
			continue
		}
		if namePid == agentPid {
			*dbusConn = name
			return nil
		}
	}
	return errors.New("Unable to determine the dbus connection name of /usr/sbin/secagentd")
}

// SetupDbusMonitor sets up dbus monitoring between secagentd and missive and
// returns the resulting DbusEventMonitor.
func SetupDbusMonitor(ctx context.Context, agentPid uint64) (func() ([]dbusutil.CalledMethod, error), error) {
	dbo, err := dbusutil.NewDBusObject(ctx, "org.freedesktop.DBus", "org.freedesktop.DBus",
		"/org/freedesktop/DBus")
	if err != nil {
		return nil, errors.Wrap(err, "unable to get DBus object")
	}

	// Trigger a synthetic process event. Since missived uses speculative queues,
	// secagentd does not establish a DBus connection until it sends the first event.
	// This generated event forces secagentd to establish the DBus connection instantly so
	// we do not time out polling for the connection name.
	if err := testexec.CommandContext(ctx, "true").Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "failed to trigger a synthetic process event")
	}

	var dbusConn string
	// secagentd may have just been restarted so Poll for a bit until it
	// establishes a dbus connection. (Batch interval can be up to 5s + load delays)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return getSecagentdDbusConn(ctx, dbo, agentPid, &dbusConn)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "timed out waiting for secagentd to be available")
	}

	// Match rules so that we only monitor EnqueueRecord method calls
	// originating from secagentd.
	m := []dbusutil.MatchSpec{{Type: "method_call",
		Path:      dbus.ObjectPath("/org/chromium/Missived"),
		Interface: "org.chromium.Missived",
		Member:    "EnqueueRecord",
		Sender:    dbusConn}}

	// Start monitoring dbus.
	return dbusutil.DbusEventMonitor(ctx, m)
}

// SetupDbusWatcherWithTimeout sets up dbus monitoring between secagentd and missive and
// returns the resulting DBusEventWatcher and the cancel function.
func SetupDbusWatcherWithTimeout(ctx context.Context, agentPid uint64, timeout time.Duration) (*dbusutil.EventWatcher, context.CancelFunc, error) {
	dbo, err := dbusutil.NewDBusObject(ctx, "org.freedesktop.DBus", "org.freedesktop.DBus",
		"/org/freedesktop/DBus")
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to get DBus object")
	}

	// Trigger a synthetic process event. Since missived uses speculative queues,
	// secagentd does not establish a DBus connection until it sends the first event.
	// This generated event forces secagentd to establish the DBus connection instantly so
	// we do not time out polling for the connection name.
	if err := testexec.CommandContext(ctx, "true").Run(testexec.DumpLogOnError); err != nil {
		return nil, nil, errors.Wrap(err, "failed to trigger a synthetic process event")
	}

	var dbusConn string
	// secagentd may have just been restarted so Poll for a bit until it
	// establishes a dbus connection. (Batch interval can be up to 5s + load delays)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return getSecagentdDbusConn(ctx, dbo, agentPid, &dbusConn)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return nil, nil, errors.Wrap(err, "timed out waiting for secagentd to be available")
	}

	// Match rules so that we only monitor EnqueueRecord method calls
	// originating from secagentd.
	m := []dbusutil.MatchSpec{{Type: "method_call",
		Path:      dbus.ObjectPath("/org/chromium/Missived"),
		Interface: "org.chromium.Missived",
		Member:    "EnqueueRecord",
		Sender:    dbusConn}}

	// Start monitoring dbus.
	evCtx, cancel := context.WithTimeout(ctx, timeout)
	ew, err := dbusutil.NewEventWatcher(evCtx, m)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrap(err, "failed to create dbus EventWatcher")
	}

	return ew, cancel, nil
}

// SetupAddMatchDbusWatcherWithTimeout sets up dbus monitoring for AddMatch signals.
// Returns the resulting DBusEventWatcher and the cancel function.
func SetupAddMatchDbusWatcherWithTimeout(ctx context.Context, timeout time.Duration) (*dbusutil.EventWatcher, context.CancelFunc, error) {
	m := []dbusutil.MatchSpec{{Type: "method_call",
		Path:      dbus.ObjectPath("/org/freedesktop/DBus"),
		Interface: "org.freedesktop.DBus",
		Member:    "AddMatch", Sender: ""}}
	evCtx, cancel := context.WithTimeout(ctx, timeout)
	ew, err := dbusutil.NewEventWatcher(evCtx, m)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrap(err, "failed to create dbus addMatchMonitor")
	}

	return ew, cancel, nil
}

// WaitForAddMatchSignal monitors the event watcher for the UserDataAuth add match signal.
func WaitForAddMatchSignal(addMatchWatcher *dbusutil.EventWatcher) error {
	for {
		event, ok := <-addMatchWatcher.Events()
		if !ok {
			return errors.New("failed to find AddMatch signal for UserDataAuth")
		}
		// Verify that it is UserDataAuth which is the relevant cryptohome dbus interface.
		if event.Arguments[0] != nil && event.Arguments[0] == "type='signal', sender='org.chromium.UserDataAuth', interface='org.chromium.UserDataAuthInterface', path='/org/chromium/UserDataAuth'" {
			return nil
		}
	}
}
