// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	pb "go.chromium.org/tast-tests/cros/services/cros/camera"
	"go.chromium.org/tast/core/errors"
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
	countStr, err := crosconfig.Get(ctx, "/camera", "count")
	if err != nil {
		return nil, err
	}
	numTotalCam, err := strconv.Atoi(countStr)
	if err != nil {
		return nil, err
	}

	// Detect builtin USB cameras.
	usbCams, err := testutil.BuiltinUsbCamerasFromV4L2Test(ctx)
	if err != nil {
		return nil, err
	}
	numUsbCam := len(usbCams)

	// Detect MIPI cameras.
	mipiCams, err := testutil.MIPICamerasFromCrOSCameraTool(ctx)
	if err != nil {
		return nil, err
	}
	numMipiCam := len(mipiCams)

	if numUsbCam+numMipiCam != numTotalCam {
		return nil, errors.Errorf("failed to enumerate all built-in cameras (found %v usb, %v mipi)", numUsbCam, numMipiCam)
	}

	return &emptypb.Empty{}, nil
}
