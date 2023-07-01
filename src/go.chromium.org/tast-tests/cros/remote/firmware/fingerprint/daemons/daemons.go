// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package daemons provides functions for stopping and later resuming
// daemons.
package daemons

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/durationpb"
)

// DaemonState holds the previous state of a daemon.
type DaemonState struct {
	name       string
	wasStarted bool // True if daemon was originally started.
}

// StopDaemons stops the specified daemons and returns their original state.
func StopDaemons(ctx context.Context, upstartService platform.UpstartServiceClient, daemons []string) ([]DaemonState, error) {
	var ret []DaemonState
	for _, name := range daemons {
		status, err := upstartService.JobStatus(ctx, &platform.JobStatusRequest{JobName: name})
		if err != nil {
			return ret, errors.Wrap(err, "failed to get status for "+name)
		}

		daemonWasStarted := upstart.Goal(status.GetGoal()) == upstart.StartGoal

		if daemonWasStarted {
			testing.ContextLog(ctx, "Stopping ", name)
			if _, err := upstartService.StopJob(ctx, &platform.StopJobRequest{
				JobName: name,
			}); err != nil {
				return ret, errors.Wrap(err, "failed to stop "+name)
			}
		}

		ret = append(ret, DaemonState{
			name:       name,
			wasStarted: daemonWasStarted,
		})
	}

	return ret, nil
}

// RestoreDaemons restores the daemons to the state provided in daemonState.
func RestoreDaemons(ctx context.Context, upstartService platform.UpstartServiceClient, skipWaitingForJob bool, daemons []DaemonState) error {
	var firstErr error

	for i := len(daemons) - 1; i >= 0; i-- {
		daemon := daemons[i]

		if daemon.wasStarted && !skipWaitingForJob {
			// The service can be in stop/waiting state when
			// dependencies are not satisfied yet. Let's wait for
			// the service to enter running state.
			testing.ContextLog(ctx, "Waiting for "+daemon.name+" to reach start/running state")
			_, err := upstartService.WaitForJobStatus(ctx, &platform.WaitForJobStatusRequest{
				JobName: daemon.name,
				Goal:    string(upstart.StartGoal),
				State:   string(upstart.RunningState),
				Timeout: durationpb.New(10 * time.Second),
			})
			if err == nil {
				continue
			}
			testing.ContextLog(ctx, "Wait for "+daemon.name+" finished with: ", err)
		}

		testing.ContextLog(ctx, "Checking state for ", daemon.name)
		status, err := upstartService.JobStatus(ctx, &platform.JobStatusRequest{JobName: daemon.name})
		if err != nil {
			testing.ContextLog(ctx, "Failed to get state for "+daemon.name+": ", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		testing.ContextLog(ctx, "Job "+daemon.name+" is "+status.GetGoal()+"/"+status.GetState())

		started := upstart.Goal(status.GetGoal()) == upstart.StartGoal

		if daemon.wasStarted {
			if !started {
				testing.ContextLog(ctx, "Starting ", daemon.name)
				// StartJob blocks until job enters 'running'
				// state.
				_, err := upstartService.StartJob(ctx, &platform.StartJobRequest{
					JobName: daemon.name,
				})
				if err != nil {
					testing.ContextLog(ctx, "Failed to start "+daemon.name+": ", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		} else {
			if started {
				testing.ContextLog(ctx, "Stopping ", daemon.name)
				_, err := upstartService.StopJob(ctx, &platform.StopJobRequest{
					JobName: daemon.name,
				})
				if err != nil {
					testing.ContextLog(ctx, "Failed to stop "+daemon.name+": ", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		}
	}
	return firstErr
}
