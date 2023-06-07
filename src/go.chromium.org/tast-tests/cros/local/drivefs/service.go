// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import (
	"context"

	"google.golang.org/grpc"

	dpb "go.chromium.org/tast-tests/cros/services/cros/drivefs"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			dpb.RegisterDriveFsServiceServer(srv, &Service{s})
		},
	})
}

// Service provides a way to remotely execute methods on DriveFS.
type Service struct {
	s *testing.ServiceState
}

// WaitForMount is called when a remote DUT wants to wait for the DriveFS mount
// point associated with the username supplied has finished mounting.
func (f *Service) WaitForMount(ctx context.Context, request *dpb.WaitForMountRequest) (*dpb.WaitForMountResponse, error) {
	mountPath, err := WaitForDriveFs(ctx, request.GetUsername())
	if err != nil {
		return nil, err
	}

	return &dpb.WaitForMountResponse{MountPath: mountPath}, nil
}
