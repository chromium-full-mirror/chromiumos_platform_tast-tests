// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package daemons provides functions for stopping and later resuming
// daemons.
package daemons

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/durationpb"
)

// UpstartJob references the job name and any unique args, together, that
// uniquely identify the job instance.
type UpstartJob struct {
	// Name of the job.
	Name string
	// Args for the the job instance.
	Args map[string]string
}

// requestArgs return Args in the grpc request format.
func (j UpstartJob) requestArgs() []*platform.Arg {
	var args = make([]*platform.Arg, 0, len(j.Args))
	for k, v := range j.Args {
		args = append(args, &platform.Arg{
			Key:   k,
			Value: v,
		})
	}
	return args
}

// String produces a human readable version of an UpstartJob.
func (j UpstartJob) String() string {
	var s strings.Builder
	s.WriteString(j.Name)
	if len(j.Args) > 0 {
		args := make([]string, 0, len(j.Args))
		for k, v := range j.Args {
			args = append(args, k+"="+v)
		}
		s.WriteString("(" + strings.Join(args, " ") + ")")
	}
	return s.String()
}

// DaemonState holds the previous state of a daemon.
type DaemonState struct {
	job        UpstartJob
	wasStarted bool // True if daemon was originally started.
}

// StopDaemons stops the specified daemons/jobs and returns their original states.
func StopDaemons(ctx context.Context, upstartService platform.UpstartServiceClient, jobs []UpstartJob) ([]DaemonState, error) {
	var states []DaemonState
	for _, job := range jobs {
		status, err := upstartService.JobStatus(ctx, &platform.JobStatusRequest{
			JobName: job.Name,
			Args:    job.requestArgs(),
		})
		if err != nil {
			return states, errors.Wrap(err, "failed to get status for "+job.String())
		}

		daemonWasStarted := upstart.Goal(status.GetGoal()) == upstart.StartGoal

		if daemonWasStarted {
			testing.ContextLog(ctx, "Stopping ", job)
			if _, err := upstartService.StopJob(ctx, &platform.StopJobRequest{
				JobName: job.Name,
				Args:    job.requestArgs(),
			}); err != nil {
				return states, errors.Wrap(err, "failed to stop "+job.String())
			}
		}

		states = append(states, DaemonState{
			job:        job,
			wasStarted: daemonWasStarted,
		})
	}

	return states, nil
}

// RestoreDaemons restores the daemons to the state provided in states.
func RestoreDaemons(ctx context.Context, upstartService platform.UpstartServiceClient, skipWaitingForJob bool, oldStates []DaemonState) error {
	var firstErr error

	for i := len(oldStates) - 1; i >= 0; i-- {
		state := oldStates[i]

		if state.wasStarted && !skipWaitingForJob {
			// The service can be in stop/waiting state when
			// dependencies are not satisfied yet. Let's wait for
			// the service to enter running state.
			testing.ContextLog(ctx, "Waiting for ", state.job, " to reach start/running state")
			_, err := upstartService.WaitForJobStatus(ctx, &platform.WaitForJobStatusRequest{
				JobName: state.job.Name,
				Args:    state.job.requestArgs(),
				Goal:    string(upstart.StartGoal),
				State:   string(upstart.RunningState),
				Timeout: durationpb.New(10 * time.Second),
			})
			if err == nil {
				continue
			}
			testing.ContextLog(ctx, "Wait for ", state.job, " finished with: ", err)
		}

		testing.ContextLog(ctx, "Checking state for ", state.job)
		status, err := upstartService.JobStatus(ctx, &platform.JobStatusRequest{
			JobName: state.job.Name,
			Args:    state.job.requestArgs(),
		})
		if err != nil {
			testing.ContextLog(ctx, "Failed to get state for ", state.job, ": ", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		testing.ContextLog(ctx, "Job ", state.job, " is ",
			status.GetGoal(), "/", status.GetState())

		started := upstart.Goal(status.GetGoal()) == upstart.StartGoal

		if state.wasStarted {
			if !started {
				testing.ContextLog(ctx, "Starting ", state.job)
				// StartJob blocks until job enters 'running'
				// state.
				_, err := upstartService.StartJob(ctx, &platform.StartJobRequest{
					JobName: state.job.Name,
					Args:    state.job.requestArgs(),
				})
				if err != nil {
					testing.ContextLog(ctx, "Failed to start ", state.job, ": ", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		} else {
			if started {
				testing.ContextLog(ctx, "Stopping ", state.job)
				_, err := upstartService.StopJob(ctx, &platform.StopJobRequest{
					JobName: state.job.Name,
					Args:    state.job.requestArgs(),
				})
				if err != nil {
					testing.ContextLog(ctx, "Failed to stop ", state.job, ": ", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		}
	}
	return firstErr
}
