// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			platform.RegisterUpstartServiceServer(srv, &UpstartService{s})
		},
	})
}

// UpstartService implements tast.cros.platform.UpstartService.
type UpstartService struct {
	s *testing.ServiceState
}

// CheckJob validates that the given upstart job is running.
func (*UpstartService) CheckJob(ctx context.Context, request *platform.CheckJobRequest) (*empty.Empty, error) {
	args := unpackRequestArgs(request.GetArgs())
	return &empty.Empty{}, upstart.CheckJob(ctx, request.JobName, args...)
}

// JobStatus returns the current status of job.
// If the PID is unavailable (i.e. the process is not running), 0 will be returned.
// An error will be returned if the job is unknown (i.e. it has no config in /etc/init).
func (*UpstartService) JobStatus(ctx context.Context, request *platform.JobStatusRequest) (*platform.JobStatusResponse, error) {
	args := unpackRequestArgs(request.GetArgs())
	goal, state, pid, err := upstart.JobStatus(ctx, request.JobName, args...)
	return &platform.JobStatusResponse{
		Goal:  string(goal),
		State: string(state),
		Pid:   int32(pid),
	}, err
}

// StartJob starts job. If it is already running, this returns an error.
func (*UpstartService) StartJob(ctx context.Context, request *platform.StartJobRequest) (*empty.Empty, error) {
	args := unpackRequestArgs(request.GetArgs())
	return &empty.Empty{}, upstart.StartJob(ctx, request.JobName, args...)
}

// StopJob stops job. If it is not currently running, this is a no-op.
func (*UpstartService) StopJob(ctx context.Context, request *platform.StopJobRequest) (*empty.Empty, error) {
	args := unpackRequestArgs(request.GetArgs())
	return &empty.Empty{}, upstart.StopJob(ctx, request.JobName, args...)
}

// RestartJob restarts the job (single-instance) or the specified instance of
// the job (multiple-instance).
func (*UpstartService) RestartJob(ctx context.Context, request *platform.RestartJobRequest) (*empty.Empty, error) {
	args := unpackRequestArgs(request.GetArgs())
	return &empty.Empty{}, upstart.RestartJob(ctx, request.JobName, args...)
}

// EnableJob enables an upstart job that was previously disabled.
func (*UpstartService) EnableJob(ctx context.Context, request *platform.EnableJobRequest) (*empty.Empty, error) {
	return &empty.Empty{}, upstart.EnableJob(ctx, request.JobName)
}

// DisableJob disables an upstart job, which takes effect on the next reboot.
func (*UpstartService) DisableJob(ctx context.Context, request *platform.DisableJobRequest) (*empty.Empty, error) {
	return &empty.Empty{}, upstart.DisableJob(ctx, request.JobName)
}

// IsJobEnabled checks if the given upstart job is enabled.
func (*UpstartService) IsJobEnabled(ctx context.Context, request *platform.IsJobEnabledRequest) (*platform.IsJobEnabledResponse, error) {
	enabled, err := upstart.IsJobEnabled(request.JobName)
	return &platform.IsJobEnabledResponse{Enabled: enabled}, err
}

// WaitForJobStatus waits for the given upstart job to have the status described by goal/state.
func (*UpstartService) WaitForJobStatus(ctx context.Context, request *platform.WaitForJobStatusRequest) (*empty.Empty, error) {
	args := unpackRequestArgs(request.GetArgs())
	return &empty.Empty{}, upstart.WaitForJobStatus(ctx,
		request.JobName,
		upstartcommon.Goal(request.Goal),
		upstartcommon.State(request.State),
		upstart.TolerateWrongGoal,
		request.Timeout.AsDuration(),
		args...,
	)
}

// unpackRequestArgs unpacks the grpc request Arg slice into an upstart.Arg
// slice.
func unpackRequestArgs(requestArgs []*platform.Arg) []upstart.Arg {
	var args []upstart.Arg
	for _, arg := range requestArgs {
		args = append(args, upstart.WithArg(arg.GetKey(), arg.GetValue()))
	}
	return args
}
