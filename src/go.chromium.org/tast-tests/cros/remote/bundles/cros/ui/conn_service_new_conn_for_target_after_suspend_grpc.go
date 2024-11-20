// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnServiceNewConnForTargetAfterSuspendGRPC,
		Desc:         "Test re-establishing chrome connection through services after power suspension",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

// ConnServiceNewConnForTargetAfterSuspendGRPC tests ConnService's ability to re-establish
// chrome connection after power suspension.
func ConnServiceNewConnForTargetAfterSuspendGRPC(ctx context.Context, s *testing.State) {
	targetIds := createNewConns(ctx, s)
	runPowerSuspend(ctx, s)
	verifyNewConnForTarget(ctx, s, targetIds)
}

// createNewConns creates new Chrome connections and return connection targetIds.
func createNewConns(ctx context.Context, s *testing.State) (targetIds []string) {
	numConns := 3
	url := "about:blank"
	targetIds = make([]string, numConns)

	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the GRPC server on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Start Chrome on the DUT.
	cs := pb.NewChromeServiceClient(cl.Conn)
	loginReq := &pb.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	// Create new Chrome connections pointing to the same url.
	svc := pb.NewConnServiceClient(cl.Conn)
	for i := 0; i < numConns; i++ {
		res, err := svc.NewConn(ctx, &pb.NewConnRequest{Url: url})
		if err != nil {
			s.Fatalf("Failed to open page %v: %v", url, err)
		}
		targetIds[i] = res.TargetId
	}
	return targetIds
}

// runPowerSuspend executes power suspension and ensures that the DUT can be reached after DUT wakes up.
func runPowerSuspend(ctx context.Context, s *testing.State) {
	testing.ContextLog(ctx, "Before power suspension")
	cmd := s.DUT().Conn().CommandContext(ctx, "powerd_dbus_suspend", "--delay=0", "--suspend_for_sec=3")
	// Power suspension breaks the SSH connection which sometimes causes error for
	// powerd_dbus_suspend executed over SSH if it is executed as a single blocking call.
	// So we only check if the command is successfully triggered but doesn't
	// check the error code for completion.
	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to suspend: ", err)
	}
	cmd.Wait()
	testing.ContextLog(ctx, "After power suspension")

	// Re-establish SSH connection after power suspension.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if s.DUT().Connected(ctx) {
			testing.ContextLog(ctx, "DUT is connected through SSH")
			return nil
		}
		if err := s.DUT().Connect(ctx); err != nil {
			return errors.New("failed to create SSH connection")
		}
		return errors.New("succeeded in creating SSH connection")
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to re-establish SSH connection after power suspension: ", err)
	}
}

// verifyNewConnForTarget ensures that NewConnForTarget can access existing chrome connections through targetIds.
func verifyNewConnForTarget(ctx context.Context, s *testing.State, targetIds []string) {
	cl, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to connect to the GRPC server on the DUT: ", err)
	}
	defer cl.Close(ctx)

	cs := pb.NewChromeServiceClient(cl.Conn)
	svc := pb.NewConnServiceClient(cl.Conn)

	testing.ContextLog(ctx, "Reconnecting to Chrome")
	loginReq := &pb.NewRequest{TryReuseSession: true, KeepState: true}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	if _, err := cs.Reconnect(ctx, &empty.Empty{}, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to reconnect to Chrome after power suspension: ", err)
	}

	for _, targetID := range targetIds {
		newConn, err := svc.NewConnForTarget(ctx, &pb.NewConnForTargetRequest{TargetId: targetID})
		if err != nil {
			s.Fatalf("Failed when calling NewConnForTargetRequest for %v: %v", targetID, err)
		}
		activateTargetRequest := &pb.ActivateTargetRequest{
			Id: newConn.Id,
		}
		_, err = svc.ActivateTarget(ctx, activateTargetRequest)
		if err != nil {
			s.Fatalf("Failed when calling ActivateTarget for %v: %v", activateTargetRequest, err)
		}
	}
}
