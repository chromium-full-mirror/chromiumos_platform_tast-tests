// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram/histogrampb"
	cl "go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			powerpb.RegisterSuspendPerfServiceServer(srv, &SuspendPerfService{s: s})
		},
	})
}

type SuspendPerfService struct {
	s *testing.ServiceState
}

func (h *SuspendPerfService) Prepare(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {

	// Login as a test user
	cr, err := cl.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to login DUT")
	}
	defer cr.Close(ctx)

	err = cr.FinishUserLogin(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for login DUT")
	}

	return &empty.Empty{}, nil
}

func (h *SuspendPerfService) GetHistogram(ctx context.Context, req *powerpb.HistogramRequest) (*histogrampb.Histogram, error) {
	void := &histogrampb.Histogram{}
	name := req.GetName()

	// This will get the histogram from current running Chrome.
	cr, err := cl.New(ctx, cl.KeepState(), cl.ForceReuseSession())
	if err != nil {
		return void, errors.Wrap(err, "failed to connect to Chrome")
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return void, errors.Wrap(err, "failed to get test API connection")
	}

	hist, err := metrics.GetHistogram(ctx, tconn, name)
	if err != nil {
		return void, errors.Wrap(err, "failed to get a histogram")
	}

	return hist.Proto(), nil
}
