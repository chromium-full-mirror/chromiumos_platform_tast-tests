// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dbus

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: EventWatcher,
		Desc: "Check dbusutil.EventWatcher",
		Contacts: []string{
			"chromeos-audio-bugs@google.com",
			"aaronyu@google.com",
		},
		BugComponent: "b:776546", // ChromeOS > Platform > Technologies > Audio > Test
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
		},
		Timeout:      time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func EventWatcher(ctx context.Context, s *testing.State) {
	const (
		iface             = "org.freedesktop.DBus"
		monitoredMethod   = "ListNames"
		unmonitoredMethod = "ListActivatableNames"
	)

	conn, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("Failed to get system bus: ", err)
	}

	ew, err := dbusutil.NewEventWatcher(ctx, []dbusutil.MatchSpec{
		{
			Interface: iface,
			Member:    monitoredMethod,
		},
	})
	if err != nil {
		s.Fatal("Cannot create EventWatcher: ", err)
	}
	defer ew.Close()

	if err := conn.BusObject().CallWithContext(ctx, iface+"."+monitoredMethod, 0).Err; err != nil {
		s.Fatalf("Failed to call %s: %s", monitoredMethod, err)
	}

	select {
	case <-ew.Events():
	case <-time.After(time.Second):
		s.Fatalf("Did not find event for %s", monitoredMethod)
	}

	if err := conn.BusObject().CallWithContext(ctx, iface+"."+unmonitoredMethod, 0).Err; err != nil {
		s.Fatalf("Failed to call %s: %s", monitoredMethod, err)
	}

	select {
	case <-ew.Events():
		s.Fatalf("Found event for %s but it is not called", monitoredMethod)
	case <-time.After(time.Second):
	}

	ew.Close()
	select {
	case _, ok := <-ew.Events():
		if ok {
			s.Error("ew.Events() still open after ew.Close()")
		}
	case <-time.After(time.Second):
		s.Fatal("ew.Events() blocked after ew.Close()")
	}

	if err := ew.Err(); err != dbusutil.ErrEventWatcherClosed {
		s.Error("ev.Err() is not ErrEventWatcherClosed after Close()")
	}
}
