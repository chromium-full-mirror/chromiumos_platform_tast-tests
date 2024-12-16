// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/tast-tests/cros/common/firmware/serial"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterSerialPortServiceServer(srv, &SerialPortService{s: s})
		},
	})
}

// SerialPortService implements tast.cros.firmware.SerialPortService
type SerialPortService struct {
	s        *testing.ServiceState
	ports    map[uint32]serial.Port
	nextPort uint32
}

// Open handles the Open rpc call.
func (s *SerialPortService) Open(ctx context.Context, in *pb.SerialPortConfig) (*pb.PortId, error) {
	testing.ContextLog(ctx, "Opening service port")

	readTimeoutProto := in.GetReadTimeout()
	err := readTimeoutProto.CheckValid()
	if err != nil {
		return nil, errors.Wrap(err, "converting ReadTimeout")
	}
	readTimeout := readTimeoutProto.AsDuration()
	p, err := serial.NewConnectedPortOpener(in.GetName(), int(in.GetBaud()), readTimeout).OpenPort(ctx)
	if err != nil {
		return nil, err
	}

	if s.ports == nil {
		s.ports = make(map[uint32]serial.Port)
		s.nextPort = 1
	}
	id := s.nextPort
	s.nextPort++
	s.ports[id] = p
	return &pb.PortId{Value: id}, nil
}

func (s *SerialPortService) getPort(id uint32) (serial.Port, error) {
	if s.ports == nil {
		return nil, errors.New("no ports have been opened")
	}

	p, ok := s.ports[id]
	if !ok {
		return nil, errors.Errorf("port %d not found", id)
	}

	return p, nil
}

// Read handles the Read rpc call.
func (s *SerialPortService) Read(ctx context.Context, in *pb.SerialReadRequest) (*wrapperspb.BytesValue, error) {
	p, err := s.getPort(in.GetId().GetValue())
	if err != nil {
		return nil, err
	}
	buf := make([]byte, in.GetMaxLen())
	readLen, err := p.Read(ctx, buf)
	if err != nil {
		return nil, err
	}
	return &wrapperspb.BytesValue{Value: buf[:readLen]}, err
}

// Write handles the Write rpc call.
func (s *SerialPortService) Write(ctx context.Context, in *pb.SerialWriteRequest) (*wrapperspb.Int64Value, error) {
	p, err := s.getPort(in.GetId().GetValue())
	if err != nil {
		return nil, err
	}
	n, err := p.Write(ctx, in.GetBuffer())
	if err != nil {
		return nil, err
	}
	return &wrapperspb.Int64Value{Value: int64(n)}, err
}

// Flush handles the Flush rpc call.
func (s *SerialPortService) Flush(ctx context.Context, in *pb.PortId) (*emptypb.Empty, error) {
	p, err := s.getPort(in.GetValue())
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, p.Flush(ctx)
}

// Close handles the Close rpc call.
func (s *SerialPortService) Close(ctx context.Context, in *pb.PortId) (*emptypb.Empty, error) {
	id := in.GetValue()
	p, err := s.getPort(id)
	if err != nil {
		return nil, err
	}
	if err = p.Close(ctx); err != nil {
		return nil, err
	}
	delete(s.ports, id)
	return &emptypb.Empty{}, nil
}
