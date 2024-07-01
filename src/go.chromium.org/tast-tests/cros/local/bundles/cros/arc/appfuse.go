// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Appfuse,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Make sure arc-appfuse-provider works",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"android_container", "chrome"},
		Fixture:      "arcBooted",
	})
}
func Appfuse(ctx context.Context, s *testing.State) {
	const (
		dbusName      = "org.chromium.ArcAppfuseProvider"
		dbusPath      = "/org/chromium/ArcAppfuseProvider"
		dbusInterface = "org.chromium.ArcAppfuseProvider"
	)

	// We need to run the DBus methods as chronos because they are allowed only for chronos. See "arc/appfuse/org.chromium.ArcAppfuseProvider.conf".
	conn, obj, err := dbusutil.ConnectPrivateWithAuth(ctx, sysutil.ChronosUID, dbusName, dbus.ObjectPath(dbusPath))
	if err != nil {
		s.Fatal("Failed to connect to ", dbusName, ": ", err)
	}
	defer conn.Close()

	uid := uint32(12345)
	mountID := int32(678)
	var fd dbus.UnixFD
	if err := obj.CallWithContext(ctx, dbusInterface+".Mount", 0, uid, mountID).Store(&fd); err != nil {
		s.Fatal("Failed to mount Appfuse: ", err)
	}
	defer unix.Close(int(fd))

	if err := obj.CallWithContext(ctx, dbusInterface+".Unmount", 0, uid, mountID).Err; err != nil {
		s.Fatal("Failed to mount Appfuse: ", err)
	}
}
