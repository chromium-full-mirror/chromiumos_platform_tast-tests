// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wifi for wifi Tast tests
package wifi

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/profiler"
	pb "chromiumos/tast/services/cros/wifi"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterHPTServiceServer(srv, &HPTService{s: s})
		},
	})
}

// HPTService is for starting HPT on DUT during remote tests
type HPTService struct {
	s           *testing.ServiceState
	runningProf *profiler.RunningProf
}

var sampleRateHz int32 = 4000 // perf record default is 4000 Hz
var outFile = "/var/tmp/"

func (c *HPTService) StartHPT(ctx context.Context, req *pb.HPTRequest) (*empty.Empty, error) {

	sampleRateHz = req.SampleRateHz
	outFile = req.OutDir

	profs := []profiler.Profiler{
		// Equivalent to 'perf record -g -a -e cycles'
		profiler.Perf(profiler.PerfRecordOpts("cycles",
			&profiler.PerfRecordSamplingRate{RateType: profiler.PerfRecordFrequency, Value: int(sampleRateHz)},
			profiler.PerfRecordCallgraph)),
	}

	var err error
	if c.runningProf, err = profiler.Start(c.s.ServiceContext(), outFile, profs...); err != nil {
		return nil, errors.Wrap(err, "failed to start trace")
	}
	testing.ContextLog(ctx, "Started HPT trace")

	return &empty.Empty{}, nil
}

func (c *HPTService) StopHPT(ctx context.Context, req *pb.HPTRequest) (*empty.Empty, error) {

	if err := c.runningProf.End(c.s.ServiceContext()); err != nil {
		return nil, errors.Wrap(err, "failed to stop trace")
	}
	testing.ContextLog(ctx, "Stopped HPT trace, outfile: "+outFile)

	return &empty.Empty{}, nil
}
