// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracing

import (
	"context"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/tracing"
	pb "go.chromium.org/tast-tests/cros/services/cros/tracing"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterPerfettoTraceServiceServer(srv, &PerfettoTraceService{s: s})
		},
	})
}

// PerfettoTraceService implements the system-wide perfetto tracing service.
// User should start this service with Start() with PerfettoOption, or Attach()
// with PerfettoSessionToken.
// If PerfettoOption.Background is set, that is able to be reconnected later with
// a session token.
// After running some workload, user needs to call Stop() to stop tracing.
// Finally, user needs to call Finalize() to remove temporary files.
type PerfettoTraceService struct {
	s *testing.ServiceState

	sess *tracing.Session
}

// Start starts a session with given configuration.
// If you specify config file path in the PerfettoOption, you must put a
// perfetto config file at there on DUT before start this session. On the
// other hand, if you specify config data itself, the session creates a
// temporary config file and use it.
func (s *PerfettoTraceService) Start(ctx context.Context, req *pb.PerfettoOption) (*pb.PerfettoResponse, error) {
	if s.sess != nil {
		return nil, errors.New("this service already connected to a session")
	}
	opts := tracing.EmptyTraceSessionOptions()
	if req.TraceDataPath != "" {
		opts = append(opts, tracing.WithTraceDataPath(req.TraceDataPath))
	}
	if req.CompressTraceData {
		if req.TraceDataPath == "" {
			return nil, errors.New("CompressTraceData option requires TraceDataPath")
		}
		opts = append(opts, tracing.WithCompression())
	}
	if req.Background {
		opts = append(opts, tracing.InBackground())
	}
	config := req.GetFilePath()
	if config == "" {
		if req.GetData() == "" {
			return nil, errors.New("either config file path or config data is required")
		}
		f, err := os.CreateTemp("", "perfettotrace-*.pbtxt")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create a temporary config file")
		}
		config = f.Name()
		if _, err := f.WriteString(req.GetData()); err != nil {
			os.Remove(config)
			return nil, errors.Wrap(err, "failed to write config data")
		}
		f.Sync()
	}
	defer os.Remove(config)

	sess, err := tracing.StartSession(ctx, config, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start new trace session")
	}
	var token *pb.PerfettoSessionToken = nil
	if req.Background {
		token, err = sess.Token()
		if err != nil {
			sess.Stop(ctx)
			sess.Finalize(ctx)
			return nil, errors.Wrap(err, "failed to get token for the session")
		}
	}
	s.sess = sess

	return &pb.PerfettoResponse{TraceDataPath: sess.TraceDataPath(), Token: token}, nil
}

// Stop stops a perfetto tracing session.
func (s *PerfettoTraceService) Stop(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if s.sess == nil {
		return nil, errors.New("this is not started nor connected to any session")
	}
	if err := s.sess.Stop(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to stop trace session")
	}
	return &emptypb.Empty{}, nil
}

// Finalize removes temporary recorded files and remove session.
func (s *PerfettoTraceService) Finalize(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if s.sess == nil {
		return nil, errors.New("this is not started nor connected to any session")
	}
	if err := s.sess.Finalize(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to finalize trace session")
	}
	s.sess = nil
	return &emptypb.Empty{}, nil
}

// Reconnect reconnects a running background session.
func (s *PerfettoTraceService) Reconnect(ctx context.Context, req *pb.PerfettoSessionToken) (*empty.Empty, error) {
	if s.sess != nil {
		return nil, errors.New("this service already connected to a session")
	}
	sess, err := tracing.ReconnectBackgroundSession(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconnect a session")
	}
	s.sess = sess
	return &emptypb.Empty{}, nil
}
