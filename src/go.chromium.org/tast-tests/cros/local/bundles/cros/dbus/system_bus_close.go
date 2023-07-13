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
		Func: SystemBusClose,
		Desc: "Check dbusutil.SystemBus() returns a new connection if closed",
		Contacts: []string{
			"chromeos-audio-bugs@google.com",
			"aaronyu@google.com",
		},
		BugComponent: "b:776546", // ChromeOS > Platform > Technologies > Audio > Test
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// SystemBusClose checks that dbusutil.SystemBus returns a new connection if closed.
func SystemBusClose(ctx context.Context, s *testing.State) {
	conn1, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("dbusutil.SystemBus() failed: ", err)
	}
	if err := conn1.Close(); err != nil {
		s.Fatal("conn1.Close() failed: ", err)
	}
	if connected := conn1.Connected(); connected {
		s.Fatal("conn1 still Connected() after Close()")
	}
	conn2, err := dbusutil.SystemBus()
	if err != nil {
		s.Fatal("Second dbusutil.SystemBus() failed: ", err)
	}
	if conn1 == conn2 {
		s.Fatal("dbusutil.SystemBus() did not return a new connection after close")
	}
}
