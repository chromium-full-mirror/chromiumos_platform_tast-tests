// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/audio"
	pb "go.chromium.org/tast-tests/cros/services/cros/audio"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterCrasControlServiceServer(srv, &CrasControlService{
				s: s,
			})
		},
	})
}

// CrasControlService implements CrasControlServiceServer.
// Provides methods for interacting with the cras control dbus interface.
//
// Methods are added on an as-needed basis.
type CrasControlService struct {
	s *testing.ServiceState
}

// RestartCras restarts the cras job and waits for it to be back up before
// returning.
func (s *CrasControlService) RestartCras(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	if _, err := audio.RestartCras(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to restart cras")
	}
	return &emptypb.Empty{}, nil
}

// SetForceHfpSwbEnabled sets the state to enable or disable bluetooth HFP SWB.
func (s *CrasControlService) SetForceHfpSwbEnabled(ctx context.Context, request *pb.SetForceHfpSwbEnabledRequest) (*emptypb.Empty, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	if err := crasClient.SetForceHFPSwbEnabled(ctx, request.ForceHfpSwbEnabled); err != nil {
		return nil, errors.Wrapf(err, "failed to call SetForceHFPSwbEnabled(%t)", request.ForceHfpSwbEnabled)
	}
	return &emptypb.Empty{}, nil
}

// GetForceHfpSwbEnabled gets the force_hfp_swb_enabled state.
func (s *CrasControlService) GetForceHfpSwbEnabled(ctx context.Context, empty *emptypb.Empty) (*pb.GetForceHfpSwbEnabledResponse, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	enabled, err := crasClient.GetForceHFPSwbEnabled(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call GetForceHFPSwbEnabled()")
	}
	return &pb.GetForceHfpSwbEnabledResponse{
		ForceHfpSwbEnabled: enabled,
	}, nil
}

// GetNodes returns node information from Cras as returned by GetNodes.
func (s *CrasControlService) GetNodes(ctx context.Context, empty *emptypb.Empty) (*pb.GetNodesResponse, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	nodes, err := crasClient.GetNodes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call GetNodes()")
	}
	resp := &pb.GetNodesResponse{}
	for _, node := range nodes {
		marshalledNode, err := node.ToProto()
		if err != nil {
			return nil, err
		}
		resp.Nodes = append(resp.Nodes, marshalledNode)
	}
	return resp, nil
}

// SelectedInputNode calls GetNodes and returns the active input node.
func (s *CrasControlService) SelectedInputNode(ctx context.Context, empty *emptypb.Empty) (*pb.SelectedInputNodeResponse, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	node, err := crasClient.SelectedInputNode(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get selected input node")
	}
	marshalledNode, err := node.ToProto()
	if err != nil {
		return nil, err
	}
	return &pb.SelectedInputNodeResponse{
		SelectedInputNode: marshalledNode,
	}, nil
}

// SelectedOutputNode calls GetNodes and returns the active output node.
func (s *CrasControlService) SelectedOutputNode(ctx context.Context, empty *emptypb.Empty) (*pb.SelectedOutputNodeResponse, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	node, err := crasClient.SelectedOutputNode(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get selected output node")
	}
	marshalledNode, err := node.ToProto()
	if err != nil {
		return nil, err
	}
	return &pb.SelectedOutputNodeResponse{
		SelectedOutputNode: marshalledNode,
	}, nil
}

// SetActiveInputNode calls SetActiveInputNode with the given node ID.
func (s *CrasControlService) SetActiveInputNode(ctx context.Context, request *pb.SetActiveInputNodeRequest) (*emptypb.Empty, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	if err := crasClient.SetActiveInputNode(ctx, request.NodeId); err != nil {
		return nil, errors.Wrapf(err, "failed to set active input node with node ID %d", request.NodeId)
	}
	return &emptypb.Empty{}, nil
}

// SetActiveOutputNode calls SetActiveOutputNode with the given node ID.
func (s *CrasControlService) SetActiveOutputNode(ctx context.Context, request *pb.SetActiveOutputNodeRequest) (*emptypb.Empty, error) {
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new cras control dbus client")
	}
	if err := crasClient.SetActiveOutputNode(ctx, request.NodeId); err != nil {
		return nil, errors.Wrapf(err, "failed to set active output node with node ID %d", request.NodeId)
	}
	return &emptypb.Empty{}, nil
}
