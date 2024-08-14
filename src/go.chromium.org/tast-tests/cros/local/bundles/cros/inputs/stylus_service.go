// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/local/input"
	pb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

const (
	// 10% was chosen as the low battery threshold, however this was mostly arbitrary.
	lowStylusBatteryLevel = 10
)

// StylusService implements tast.cros.inputs.StylusService.
type StylusService struct {
	s *testing.ServiceState
}

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterStylusServiceServer(srv, &StylusService{s: s})
		},
	})
}

// FindPhysicalStylus iterates over devices, and returns the path for a physical stylus if one exists.
func (svc *StylusService) FindPhysicalStylus(ctx context.Context, req *empty.Empty) (*pb.FindPhysicalStylusResponse, error) {
	stylusFound, stylusPath, err := input.FindPhysicalStylus(ctx)
	if err != nil {
		return nil, err
	}
	if !stylusFound {
		return nil, errors.New("no stylus could be found")
	}
	return &pb.FindPhysicalStylusResponse{StylusPath: stylusPath}, nil
}

// CheckPhysicalStylusBatteryLevel checks that the stylus battery level is not below the threshold,
// if the stylus has a battery. The battery level is not updated until the stylus physically
// touches the DUT. Therefore the battery level check has to occur after the motion has been completed.
func (svc *StylusService) CheckPhysicalStylusBatteryLevel(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	stylusHasBattery, batteryLevel, err := input.FindPhysicalStylusBatteryLevel(ctx)
	if err != nil {
		return nil, err
	}

	if stylusHasBattery {
		testing.ContextLogf(ctx, "Current stylus battery: %v%%", batteryLevel)
		// Fail all tests with a battery level below the low battery threshold.
		if batteryLevel < lowStylusBatteryLevel {
			return nil, errors.Errorf("stylus battery is %v%%, this is below the low battery threshold(%v%%)", batteryLevel, lowStylusBatteryLevel)
		}
	}

	return &empty.Empty{}, nil
}

// FindPhysicalStylusBatteryLevel returns the battery level of a physical stylus if one is present.
// The battery level is not updated until the stylus physically touches the DUT.
func (svc *StylusService) FindPhysicalStylusBatteryLevel(ctx context.Context, req *empty.Empty) (*pb.FindPhysicalStylusBatteryLevelResponse, error) {
	stylusHasBattery, batteryLevel, err := input.FindPhysicalStylusBatteryLevel(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.FindPhysicalStylusBatteryLevelResponse{HasBattery: stylusHasBattery, BatteryLevel: int32(batteryLevel)}, nil
}
