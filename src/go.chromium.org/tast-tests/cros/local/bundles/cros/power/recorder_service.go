// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	pl "go.chromium.org/tast-tests/cros/local/power"
	ps "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			ps.RegisterRecorderServiceServer(srv, &RecorderService{s: s})
		},
	})
}

// RecorderService implements recorder service.
type RecorderService struct {
	s        *testing.ServiceState
	recorder *pl.Recorder
}

// Create a new recorder.
func (r *RecorderService) Create(ctx context.Context, req *ps.RecorderRequest) (*empty.Empty, error) {
	if r.recorder != nil {
		return nil, errors.New("cannot create a new recorder as one already exists")
	}

	// Outdir and testname fields are not used in this implementation, and this
	// implementation does not save test results locally on the DUT.
	r.recorder = pl.NewRecorder(ctx, time.Duration(req.IntervalSec)*time.Second, "", "")
	return &empty.Empty{}, nil
}

// Cooldown device and wait for CPU utilisation to reduce to an acceptable level.
func (r *RecorderService) Cooldown(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if r.recorder == nil {
		return nil, errors.New("cannot cooldown device as a recorder was not created")
	}

	if err := r.recorder.Cooldown(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to cooldown device")
	}

	return &empty.Empty{}, nil
}

// Start metrics recording.
func (r *RecorderService) Start(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if r.recorder == nil {
		return nil, errors.New("cannot start recording as a recorder was not created")
	}

	if err := r.recorder.Start(r.s.ServiceContext()); err != nil {
		return nil, errors.Wrap(err, "failed to start metrics recording")
	}

	return &empty.Empty{}, nil
}

// Stop metrics recording and return perf values, one-time metrics and device info.
func (r *RecorderService) Stop(ctx context.Context, req *empty.Empty) (*ps.RecorderResult, error) {
	if r.recorder == nil {
		return nil, errors.New("cannot stop recording as a recorder was not created")
	}

	perfVals, err := r.recorder.Stop(r.s.ServiceContext())
	if err != nil {
		return nil, errors.Wrap(err, "failed to stop metrics recording")
	}

	return &ps.RecorderResult{
		PerfMetrics:    perfVals.Proto(),
		OneTimeMetrics: pl.CollectOneTimeMetrics(ctx),
		DeviceInfo:     pl.GetDeviceInfo(ctx),
	}, nil
}

// Close a recorder.
func (r *RecorderService) Close(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if r.recorder == nil {
		return nil, errors.New("cannot close a recorder as one does not exist")
	}

	if err := r.recorder.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close recorder")
	}

	r.recorder = nil
	return &empty.Empty{}, nil
}
