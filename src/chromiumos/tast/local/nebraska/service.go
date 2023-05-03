// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nebraska

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/services/cros/nebraska"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			nebraska.RegisterServiceServer(srv, &Service{s: s})
		},
	})
}

// Service implements tast.cros.nebraska.Service.
type Service struct {
	s *testing.ServiceState

	instance *Nebraska
}

// Start starts a Nebraska service instance with the given parameters.
func (s *Service) Start(ctx context.Context, req *nebraska.StartRequest) (*nebraska.StartResponse, error) {
	var options []Option

	if req.Port != nil {
		options = append(options, Port(int(*req.Port)))
	}

	// Start the Nebraska service.
	instance, err := New(s.s.ServiceContext(),
		options...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Nebraska")
	}
	s.instance = instance

	if req.Port != nil && int(*req.Port) != instance.Port {
		if err := instance.Close(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to stop Nebraska")
		}
		return nil, errors.Errorf("Nebraska started with wrong port; want %d, got %d", int(*req.Port), instance.Port)
	}

	return &nebraska.StartResponse{
		Port:        int32(instance.Port),
		RuntimeRoot: instance.Root,
	}, nil
}

// Stop gracefully stops the previously started Nebraska instance.
func (s *Service) Stop(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if err := s.instance.Close(ctx); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// UpdatePayload configures Nebraska to serve an update from the given URL and metadata folder.
func (s *Service) UpdatePayload(ctx context.Context, req *nebraska.UpdatePayloadRequest) (*empty.Empty, error) {
	if err := s.instance.SetUpdateMetadata(ctx, req.Update.MetadataFolder); err != nil {
		return nil, err
	}

	if err := s.instance.SetUpdatePayloadsAddress(ctx, req.Update.Address); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}
