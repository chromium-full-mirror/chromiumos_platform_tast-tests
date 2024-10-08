// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"context"
	"fmt"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

type jobStatus struct {
	goal  upstartcommon.Goal
	state upstartcommon.State
	pid   int
}

func (js jobStatus) String() string {
	return fmt.Sprintf("%v/%v/pid=%v", js.goal, js.state, js.pid)
}

var _ fmt.Stringer = jobStatus{}

func queryJobStatus(ctx context.Context, job string) (jobStatus, error) {
	goal, state, pid, err := upstart.JobStatus(ctx, job)
	if err != nil {
		return jobStatus{}, err
	}

	return jobStatus{
		goal:  goal,
		state: state,
		pid:   pid,
	}, nil
}

// CrashDetector for audio services.
type CrashDetector struct {
	crasStatus jobStatus
}

// NewCrashDetector creates a new CrashDetector for audio services.
func NewCrashDetector(ctx context.Context) (*CrashDetector, error) {
	status, err := queryJobStatus(ctx, "cras")
	if err != nil {
		return nil, errors.Wrap(err, "failed to queryJobStatus")
	}
	if status.pid == 0 {
		return nil, errors.Wrapf(err, "got zero pid: %s, is cras running?", status)
	}
	return &CrashDetector{
		crasStatus: status,
	}, nil
}

// CheckCrash returns nil if there's no crash and an error otherwise.
func (cd *CrashDetector) CheckCrash(ctx context.Context) error {
	status, err := queryJobStatus(ctx, "cras")
	if err != nil {
		return errors.Wrap(err, "failed to queryJobStatus")
	}
	if status.pid == 0 {
		return errors.Wrapf(err, "got zero pid: %s, is cras running?", status)
	}
	if cd.crasStatus.pid != status.pid {
		return errors.Errorf("cras pid changed (%s => %s), did cras crash?", cd.crasStatus, status)
	}
	return nil
}
