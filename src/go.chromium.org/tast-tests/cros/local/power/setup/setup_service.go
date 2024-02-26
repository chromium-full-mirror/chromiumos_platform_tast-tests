// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DisableService stops a service if it is running.
func DisableService(ctx context.Context, name string) (CleanupCallback, error) {
	if !upstart.JobExists(ctx, name) {
		return nil, errors.Errorf("service %q does not exist", name)
	}
	goal, _, _, err := upstart.JobStatus(ctx, name)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get service status %q", name)
	}
	if goal == upstartcommon.StopGoal {
		testing.ContextLogf(ctx, "Not stopping service %q, not running", name)
		return nil, nil
	}
	testing.ContextLogf(ctx, "Stopping service %q", name)
	if err := upstart.StopJob(ctx, name); err != nil {
		return nil, errors.Wrapf(err, "unable to stop service %q", name)
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Restarting service %q", name)
		return upstart.StartJob(ctx, name)
	}, nil
}

// EnableService starts the specified upstart service if it is stopped.
// **DO NOT USE UNLESS YOU ARE SPECIFICALLY AFFECTED BY b/324513129**
// TODO(b/324513129): remove this once we have a better solution in place.
func EnableService(ctx context.Context, name string) (CleanupCallback, error) {
	if !upstart.JobExists(ctx, name) {
		return nil, errors.Errorf("service %q does not exist", name)
	}
	goal, _, _, err := upstart.JobStatus(ctx, name)
	if err != nil {
		testing.ContextLogf(ctx, "Unable to get service status %q", name)
		return nil, errors.Wrapf(err, "Unable to get service status %q", name)
	}
	if goal != upstartcommon.StopGoal {
		testing.ContextLogf(ctx, "Not starting service %q, it is not stopped", name)
		return nil, nil
	}
	testing.ContextLogf(ctx, "Starting service %q", name)
	if err := upstart.StartJob(ctx, name); err != nil {
		testing.ContextLogf(ctx, "unable to start service %q", name)
		return nil, errors.Wrapf(err, "unable to start service %q", name)
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Stopping service %q", name)
		return upstart.StopJob(ctx, name)
	}, nil
}

// DisableServiceIfExists stops a service if it is running. Unlike
// DisableService above, it does not return an error if the service does not
// exist.
func DisableServiceIfExists(ctx context.Context, name string) (CleanupCallback, error) {
	if !upstart.JobExists(ctx, name) {
		return nil, nil
	}
	return DisableService(ctx, name)
}
