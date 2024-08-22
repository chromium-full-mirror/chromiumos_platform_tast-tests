// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package actionlogger

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	pb "go.chromium.org/tast-tests/cros/services/cros/actionlogger"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterActionLoggerServiceServer(srv, &Service{s})
		},
	})
}

// Service implements tast.cros.actionlogger.ActionLoggerService.
type Service struct {
	s *testing.ServiceState
}

// Reset clears the action item list.
// This is expected to be called from the remote fixture hook.
func (*Service) Reset(ctx context.Context, request *emptypb.Empty) (*empty.Empty, error) {
	Reset(ctx)
	return &empty.Empty{}, nil
}

// Save saves the action logs.
// This is expected to be called from the remote fixture hook.
// Note: Theoretically it should work if the chrome service calls the uiauto
// library for UI automation from other remote tests, but it is out of the
// scope at the time of writing.
func (*Service) Save(ctx context.Context, request *emptypb.Empty) (*empty.Empty, error) {
	Save(ctx)
	return &empty.Empty{}, nil
}
