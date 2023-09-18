// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package uwb is the local service implementation of the GRPCs that enable remote testing for UWB.
package uwb

import (
	"context"
	"time"

	uc "go.chromium.org/tast-tests/cros/common/uwb"
	"go.chromium.org/tast-tests/cros/local/uwb"
	us "go.chromium.org/tast-tests/cros/services/cros/uwb"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Service holds the uwb DbusClient and the testing ServiceState
type Service struct {
	s      *testing.ServiceState
	client *uwb.DbusClient
}

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			us.RegisterUwbServiceServer(srv, &Service{s: s})
		},
	})
}

// NewClient calls the corresponding NewClient function in the local uwb dbus library and stores the clients instance
func (c *Service) NewClient(ctx context.Context, empty *emptypb.Empty) (*us.NewClientResponse, error) {
	response := us.NewClientResponse{
		Success: true,
	}

	client, err := uwb.NewClient(ctx)
	if err != nil {
		response.Success = false
		return &response, err
	}

	c.client = client
	return &response, nil
}

// Enable calls the corresponding Enable function in the local uwb dbus library
func (c *Service) Enable(ctx context.Context, empty *emptypb.Empty) (*uc.EnableResponse, error) {
	response, err := c.client.Enable(ctx)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// Disable calls the corresponding Disable function in the local uwb dbus library
func (c *Service) Disable(ctx context.Context, empty *emptypb.Empty) (*uc.DisableResponse, error) {
	response, err := c.client.Disable(ctx)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// InitSession calls the corresponding InitSession function in the local uwb dbus library
func (c *Service) InitSession(ctx context.Context, request *uc.InitSessionRequest) (*uc.InitSessionResponse, error) {
	response, err := c.client.InitSession(ctx, request.SessionId, request.Params)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// DeinitSession calls the corresponding DeinitSession function in the local uwb dbus library
func (c *Service) DeinitSession(ctx context.Context, request *uc.DeinitSessionRequest) (*uc.DeinitSessionResponse, error) {
	response, err := c.client.DeinitSession(ctx, request.SessionId)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// StartRanging calls the corresponding StartRanging function in the local uwb dbus library
func (c *Service) StartRanging(ctx context.Context, request *uc.StartRangingRequest) (*uc.StartRangingResponse, error) {
	response, err := c.client.StartRanging(ctx, request.SessionId)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// StopRanging calls the corresponding StopRanging function in the local uwb dbus library
func (c *Service) StopRanging(ctx context.Context, request *uc.StopRangingRequest) (*uc.StopRangingResponse, error) {
	response, err := c.client.StopRanging(ctx, request.SessionId)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// SetLoggerMode calls the corresponding SetLoggerMode function in the local uwb dbus library
func (c *Service) SetLoggerMode(ctx context.Context, request *uc.SetLoggerModeRequest) (*uc.SetLoggerModeResponse, error) {
	response, err := c.client.SetLoggerMode(ctx, request.LoggerMode)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// SessionParams calls the corresponding SessionParams function in the local uwb dbus library
func (c *Service) SessionParams(ctx context.Context, request *uc.SessionParamsRequest) (*uc.SessionParamsResponse, error) {
	response, err := c.client.SessionParams(ctx, request.SessionId)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Reconfigure calls the corresponding Reconfigure function in the local uwb dbus library
func (c *Service) Reconfigure(ctx context.Context, request *uc.ReconfigureRequest) (*uc.ReconfigureResponse, error) {
	response, err := c.client.Reconfigure(ctx, request.SessionId, request.Params)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// UpdateControllerMulticastList calls the corresponding UpdateControllerMulticastList function in the local uwb dbus library
func (c *Service) UpdateControllerMulticastList(ctx context.Context, request *uc.UpdateControllerMulticastListRequest) (*uc.UpdateControllerMulticastListResponse, error) {
	response, err := c.client.UpdateControllerMulticastList(ctx, request.SessionId, request.Action, request.Controlees)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// GetFiraRangingReports calls the corresponding GetFiraRangingReports function in the local uwb dbus library
func (c *Service) GetFiraRangingReports(ctx context.Context, request *us.FiraReportRequest) (*us.FiraReportResponse, error) {
	response, err := c.client.GetFiraRangingReports(ctx, request.SessionID, time.Duration(request.TimeInSeconds)*time.Second)

	if err != nil {
		return nil, err
	}

	rsp := &us.FiraReportResponse{
		RangingResults: response,
	}

	return rsp, nil
}
