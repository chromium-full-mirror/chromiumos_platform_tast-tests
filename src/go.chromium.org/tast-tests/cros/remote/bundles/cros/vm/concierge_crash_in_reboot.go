// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/types/known/durationpb"

	crash_service "go.chromium.org/tast-tests/cros/services/cros/crash"
	vm_service "go.chromium.org/tast-tests/cros/services/cros/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConciergeCrashInReboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test to check that vm_concierge doesn't crash during reboot",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "chibar@google.com"},
		BugComponent: "b:1248538",
		Attr:         []string{"group:mainline", "informational"},
		ServiceDeps:  []string{"tast.cros.crash.FixtureService", "tast.cros.vm.VMService"},
		SoftwareDeps: []string{"android_vm", "chrome"},
		Timeout:      10 * time.Minute,
	})
}

// ConciergeCrashInReboot tests that vm_concierge doesn't crash during reboot
func ConciergeCrashInReboot(ctx context.Context, s *testing.State) {
	const (
		systemCrashDir = "/var/spool/crash"
		regex          = `vm_concierge\.\d{8}\.\d{6}\.\d+\.\d+\.dmp`
	)

	d := s.DUT()

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	fs := crash_service.NewFixtureServiceClient(cl.Conn)

	req := crash_service.SetUpCrashTestRequest{
		Consent:  crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
		DevImage: true,
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// We don't have to remove existing dump files because they are stashed by fs.SetUp()
	if _, err := fs.SetUp(ctx, &req); err != nil {
		s.Error("Failed to set up: ", err)
		cl.Close(cleanupCtx)
		return
	}

	defer func() {
		if fs != nil {
			if _, err := fs.TearDown(cleanupCtx, &empty.Empty{}); err != nil {
				s.Error("Couldn't tear down: ", err)
			}
		}
		if cl != nil {
			cl.Close(cleanupCtx)
		}
	}()

	client := vm_service.NewVMServiceClient(cl.Conn)

	// Start ARCVM to make sure vm_concierge is running
	if _, err = client.RunARC(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run arcvm: ", err)
	}

	s.Log("Rebooting DUT")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	cl.Close(ctx)
	cl = nil
	fs = nil

	if err := d.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
	s.Log("Reconnected to DUT")

	// Need to reconnect to the gRPC server after rebooting DUT.
	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	fs = crash_service.NewFixtureServiceClient(cl.Conn)

	// Check if dump files are created
	waitReq := &crash_service.WaitForCrashFilesRequest{
		Dirs:    []string{systemCrashDir},
		Regexes: []string{regex},
		Timeout: durationpb.New(500 * time.Millisecond),
	}
	res, err := fs.WaitForCrashFiles(ctx, waitReq)
	if err == nil {
		for _, match := range res.Matches {
			if match.Regex == regex {
				if len(match.Files) > 0 {
					s.Fatal("Detected a concierge crash")
				}
			}
		}
	} else if !strings.Contains(err.Error(), "timed out while waiting for crash files") {
		s.Fatal("Failed to wait for crash files: ", err)
	}
}
