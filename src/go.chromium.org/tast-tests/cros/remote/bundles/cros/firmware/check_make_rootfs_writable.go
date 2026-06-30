// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast-tests/cros/remote/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CheckMakeRootfsWritable,
		Desc: "Test that we can make rootfs writable",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"tomhughes@chromium.org",
			"josienordrum@google.com",
		},
		// TODO(b/256910247): Add test to group:mainline once b/249564120 is fixed.
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
	})
}

func CheckMakeRootfsWritable(ctx context.Context, s *testing.State) {
	// Reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer d.Close(cleanupCtx)

	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}
	pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	rootfsIsWritable, err := sysutil.IsRootfsWritable(ctx, d.RPC())
	if err != nil {
		s.Fatal("Failed to check if rootfs is writable: ", err)
	}
	if rootfsIsWritable {
		s.Error("WARNING: The rootfs was already writable")
	} else {
		// Since MakeRootfsWritable will reboot the device, we must call
		// RPCClose/RPCDial before/after calling MakeRootfsWritable.
		if err := d.RPCClose(ctx); err != nil {
			s.Fatal("Failed to close rpc: ", err)
		}
		// Note that there is no way for this test to restore the rootfs back to read-only mode.
		// b/249564120 ensures that the test infrastructure will do so.
		if err := sysutil.MakeRootfsWritable(ctx, d.DUT(), d.RPCHint()); err != nil {
			s.Fatal("Failed to make rootfs writable: ", err)
		}
		if err := d.RPCDial(ctx); err != nil {
			s.Fatal("Failed to redial rpc: ", err)
		}
	}

	fs := dutfs.NewClient(d.RPC().Conn)
	const filename string = "/testfile.txt"
	content := []byte("This is a test file!")
	if err := fs.WriteFile(ctx, filename, content, 0); err != nil {
		s.Fatal("Failed to create test file: ", err)
	}
	if err := fs.Remove(cleanupCtx, filename); err != nil {
		s.Fatal("Failed to remove test file: ", err)
	}
}
