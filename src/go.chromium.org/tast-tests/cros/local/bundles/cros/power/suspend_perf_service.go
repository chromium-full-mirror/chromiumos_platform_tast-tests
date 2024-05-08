// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strconv"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/shill"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			powerpb.RegisterSuspendPerfServiceServer(srv, &SuspendPerfService{s: s})
		},
	})
}

// SuspendPerfService implements tast.cros.power.SuspendPerfService.
type SuspendPerfService struct {
	s *testing.ServiceState
}

func (h *SuspendPerfService) Suspend(ctx context.Context, req *powerpb.SuspendRequest) (*powerpb.SuspendResponse, error) {
	// Keep check_ethernet.hook away to avoid networking related issues.
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		return &powerpb.SuspendResponse{Failed: true}, errors.Wrap(err, "failed to lock the check network hook")
	}
	defer unlock()

	if out, err := testexec.CommandContext(ctx, "powerd_dbus_suspend", "--suspend_for_sec="+strconv.Itoa(int(req.Seconds))).CombinedOutput(); err != nil {
		return &powerpb.SuspendResponse{Failed: true, Output: string(out)}, errors.Wrap(err, "failed to perform system suspend")
	}

	if err := shill.WaitForOnlineAfterResume(ctx); err != nil {
		return &powerpb.SuspendResponse{Failed: true}, errors.Wrap(err, "failed to recover network")
	}

	return &powerpb.SuspendResponse{Failed: false}, nil
}
