// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tape

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/tape"
	ts "go.chromium.org/tast-tests/cros/services/cros/tape"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			ts.RegisterServiceServer(srv, &Service{})
		},
	})
}

// Service implements tast.cros.tape.Service.
type Service struct {
}

// GetDeviceID retrieves the device id from the /var/lib/devicesettings/policy.1 file.
func (service *Service) GetDeviceID(ctx context.Context, req *empty.Empty) (resp *ts.GetDeviceIDResponse, retErr error) {

	deviceID, customerID, err := tape.GetDeviceIDHelper(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve deviceID")
	}

	return &ts.GetDeviceIDResponse{DeviceID: deviceID, CustomerID: customerID}, nil
}
