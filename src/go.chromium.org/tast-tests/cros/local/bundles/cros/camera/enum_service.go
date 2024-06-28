// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/camera"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterEnumServiceServer(srv, &EnumService{s})
		},
	})
}

// EnumService implements tast.cros.camera.EnumService.
type EnumService struct {
	s *testing.ServiceState
}

// CheckBuiltinCamera checks whether built-in cameras are all enumerated (again).
func (*EnumService) CheckBuiltinCamera(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	if err := testutil.CheckBuiltinCameraEnumeration(ctx); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
