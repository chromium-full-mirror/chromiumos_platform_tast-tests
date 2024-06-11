// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PowertopRecorderOptions provides options to run PowertopRecorder.
type PowertopRecorderOptions struct {
	// Intervals to report data.
	Interval time.Duration

	// Output path for report files.
	OutDir string

	// Whether to ignore cpu scaling freq change. `powertop` could change
	// `scaling_max_freq` to lowest freq and makes test results not
	// representative. That is a fatal error. Set this to true to output an error
	// message instead.
	IgnoreCPUScale bool
}

// PowertopRecorder runs `powertop` in background to generate reports and store
// them under an output dir.
type PowertopRecorder struct {
	opts             *PowertopRecorderOptions
	args             []string
	cmd              *testexec.Cmd
	cpu0StartMaxFreq float64
}

// NewPowertopRecorder creates a new instance of PowertopRecorder.
func NewPowertopRecorder(ctx context.Context, opts *PowertopRecorderOptions) (*PowertopRecorder, error) {
	// Ensures that the output dir exists.
	if _, err := os.Stat(opts.OutDir); os.IsNotExist(err) {
		if err := os.Mkdir(opts.OutDir, 0755); err != nil {
			return nil, errors.Wrap(err, "failed to create powertop output directory")
		}
	}

	// maxTestDurationSeconds is an arbitrary long time that should be longer than
	// all tests so that `powertop` does not exit before test ends.
	const maxTestDurationSeconds = 3600

	iterations := int(maxTestDurationSeconds / opts.Interval.Seconds())

	return &PowertopRecorder{
		opts: opts,
		args: []string{
			"-C", "powertop.csv",
			"-i", fmt.Sprint(iterations),
			"-t", fmt.Sprint(int(opts.Interval.Seconds())),
		},
	}, nil
}

// Start starts `powertop` in background.
func (t *PowertopRecorder) Start(ctx context.Context) error {
	if t.cmd != nil {
		return errors.New("PowertopRecorder already started")
	}

	var err error
	t.cpu0StartMaxFreq, err = pm.CPUScalingFreq("cpu0", "max")
	if err != nil {
		return errors.Wrap(err, "failed to get cpu0 scaling_max_freq")
	}

	t.cmd = testexec.CommandContext(ctx, "powertop", t.args...)

	// Change work dir to `opts.OutDir` to write result files to it. Passing full path
	// in `-C` does not work with temp tast out dir.
	t.cmd.Cmd.Dir = t.opts.OutDir

	return t.cmd.Start()
}

// Stop stops the running `powertop`.
func (t *PowertopRecorder) Stop(ctx context.Context) error {
	if err := t.cmd.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to stop powertop")
	}
	if err := t.cmd.Wait(); err != nil {
		// SIGTERM always causes Wait() to return an error.
		testing.ContextLogf(ctx, "powertop termination wait failed (%q)", err)
	}
	t.cmd = nil

	cpu0MaxFreq, err := pm.CPUScalingFreq("cpu0", "max")
	if err != nil {
		return errors.Wrap(err, "failed to get cpu0 scaling_max_freq")
	}

	if cpu0MaxFreq != t.cpu0StartMaxFreq {
		if t.opts.IgnoreCPUScale {
			testing.ContextLogf(ctx, "CPU0 max scaling freq changed: %v -> %v", t.cpu0StartMaxFreq, cpu0MaxFreq)
		} else {
			return errors.Errorf("CPU0 max scaling freq changed: %v -> %v", t.cpu0StartMaxFreq, cpu0MaxFreq)
		}
	}

	return nil
}
