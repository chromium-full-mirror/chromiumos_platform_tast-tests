// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
	pl "go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterLocalInfoServiceServer(srv, &LocalInfoService{s: s})
		},
	})
}

// LocalInfoService implements services to get info from local DUT.
type LocalInfoService struct {
	s *testing.ServiceState
}

// GetDeviceInfoFromDUT gets device info from DUT.
func (s *LocalInfoService) GetDeviceInfoFromDUT(ctx context.Context, req *empty.Empty) (*pb.DeviceInfo, error) {
	return pl.GetDeviceInfo(ctx), nil
}

// GetOneTimeMetricsFromDUT collects battery related metrics required.
func (s *LocalInfoService) GetOneTimeMetricsFromDUT(ctx context.Context, req *empty.Empty) (*pb.OneTimeMetrics, error) {
	return pl.CollectOneTimeMetrics(ctx), nil
}
