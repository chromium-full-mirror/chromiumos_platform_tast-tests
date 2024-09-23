// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/errors"
)

// SwitchService wraps go.chromium.org/chromiumos/config/go/test/lab/api/passport.SwitchServiceServer
//
// We use this interface as an intermediate so we can create a "local" implementation for testbeds
// that have not yet been updated to use a remote service.
// If we were to use the implementation in go.chromium.org/chromiumos/config directly, then we could
// break Tast compilation when updates are auto-rolled into Tast.
type SwitchService interface {
	// GetSwitches probes all connected switches to the host device.
	GetSwitches(ctx context.Context, req *passport.GetSwitchesRequest, opts ...grpc.CallOption) (*passport.GetSwitchesResponse, error)
	// ResetAllSwitches re-initializes all found switches and sets them to the "disabled" state.
	ResetAllSwitches(ctx context.Context, req *passport.ResetAllSwitchesRequest, opts ...grpc.CallOption) (*passport.ResetAllSwitchesResponse, error)
	// ConfigureSwitchPort configures a single port on a switch.
	ConfigureSwitchPort(ctx context.Context, req *passport.ConfigureSwitchPortRequest, opts ...grpc.CallOption) (*passport.ConfigureSwitchPortResponse, error)
}

// localSwitchService implements SwitchService by wrapping allion switch
// controls for a "local" usb controlled switch connected directly to the same host running
// the test.
type localSwitchService struct {
}

// NewLocalSwitchService creates a switch service for manipulating switches connected locally to
// the host without running a separate gRPC service.
func NewLocalSwitchService(ctx context.Context) (SwitchService, error) {
	if err := utils.InitFixture(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to initialize switches on host")
	}
	return &localSwitchService{}, nil
}

// GetSwitches probes all connected switches to the host device.
func (s *localSwitchService) GetSwitches(ctx context.Context, req *passport.GetSwitchesRequest, opts ...grpc.CallOption) (*passport.GetSwitchesResponse, error) {
	var switches []*passport.SwitchFixture
	for key := range utils.GetFixtureOnline() {
		switches = append(switches, &passport.SwitchFixture{Id: key})
	}
	return &passport.GetSwitchesResponse{Switches: switches}, nil
}

// ResetAllSwitches re-initializes all found switches and sets them to the "disabled" state.
func (s *localSwitchService) ResetAllSwitches(ctx context.Context, req *passport.ResetAllSwitchesRequest, opts ...grpc.CallOption) (*passport.ResetAllSwitchesResponse, error) {
	if err := utils.CloseAllFixture(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reset switches")
	}
	return &passport.ResetAllSwitchesResponse{}, nil
}

// ConfigureSwitchPort configures a single port on a switch.
func (s *localSwitchService) ConfigureSwitchPort(ctx context.Context, req *passport.ConfigureSwitchPortRequest, opts ...grpc.CallOption) (*passport.ConfigureSwitchPortResponse, error) {
	var cmd string
	switch req.GetState() {
	case passport.SwitchPortState_SWITCH_PORT_DISABLED:
		cmd = "off"
	case passport.SwitchPortState_SWITCH_PORT_ENABLED:
		cmd = "on"
	case passport.SwitchPortState_SWITCH_PORT_FLIP:
		cmd = "flip"
	default:
		return nil, errors.Errorf("failed to configure switch port, unsupported switch state %s", req.GetState())
	}

	if err := utils.ControlFixture(ctx, req.GetSwitchId(), cmd); err != nil {
		return nil, errors.Wrap(err, "failed to configure switch")
	}

	return &passport.ConfigureSwitchPortResponse{}, nil
}
