// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting.
package secagentd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/sysutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/secagentd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FileEventsRootfs,
		Desc: "Sends and verifies that all xdr messages are received",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
			"rborzello@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"bpf", "reboot", "chrome", "shipping_kernel"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		ServiceDeps:  []string{"tast.cros.secagentd.FileEventService"},
	})
}

// FileEventsRootfs tests that writes to the rootfs generates the correct file events.
func FileEventsRootfs(ctx context.Context, s *testing.State) {
	// Make rootfs on DUT rw and then execute the remote rootfs file event test.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	if err := sysutil.MakeRootfsWritable(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to make rootfs rw: ", err)
	}
	s.Log("DUT rootfs is now rw")
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to DUT RPC service: ", err)
	}
	defer cl.Close(ctx)
	fec := pb.NewFileEventServiceClient(cl.Conn)
	if _, err = fec.TestFileEvents(ctx, &pb.TestFileEventsRequest{Name: pb.TestCase_ROOT_FS.Enum()}); err != nil {
		s.Error("Test failed: ", err)
	}

}
